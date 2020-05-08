package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
)

type TelegramBot struct {
	MediaStore            *MediaStore
	TokenGenerator        *TokenGenerator
	GlobalTokenValidity   int
	PerAlbumTokenValidity int

	WebPublicURL     string
	ChatDB           *ChatDB
	AuthorizedUsers  map[string]bool
	RetryDelay       time.Duration
	NewUpdateTimeout int
	API              *tgbotapi.BotAPI
	Commands         TelegramCommands
	Messages         TelegramMessages
}

type TelegramCommands struct {
	Help     string
	NewAlbum string
	Info     string
	Share    string
	Browse   string
}

type TelegramMessages struct {
	Forbidden        string
	Help             string
	MissingAlbumName string
	ServerError      string
	AlbumCreated     string
	DoNotUnderstand  string
	Info             string
	InfoNoAlbum      string
	NoUsername       string
	ThankYouMedia    string
	SharedAlbum      string
	SharedGlobal     string
}

func NewTelegramBot() *TelegramBot {
	bot := TelegramBot{}
	bot.AuthorizedUsers = make(map[string]bool)
	return &bot
}

func (bot *TelegramBot) StartBot(token string, debug bool) {
	var telegramBot *tgbotapi.BotAPI
	var err error

	for tryAgain := true; tryAgain; tryAgain = (err != nil) {
		telegramBot, err = tgbotapi.NewBotAPI(token)
		if err != nil {
			log.Printf("Cannot start the Telegram Bot because of '%s'. Retrying in %d seconds...", err, bot.RetryDelay/time.Second)
			time.Sleep(bot.RetryDelay)
		}
	}

	log.Printf("Authorized on account %s", telegramBot.Self.UserName)

	bot.API = telegramBot
	bot.API.Debug = debug
}

func (bot *TelegramBot) Process() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = bot.NewUpdateTimeout
	updates, _ := bot.API.GetUpdatesChan(u)
	for update := range updates {
		bot.ProcessUpdate(update)
	}
}

func (bot *TelegramBot) ProcessUpdate(update tgbotapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	if update.Message.Chat == nil || update.Message.Chat.Type != "private" {
		return
	}

	text := update.Message.Text
	username := update.Message.From.UserName

	if username == "" {
		bot.replyToCommandWithMessage(update.Message, bot.Messages.NoUsername)
		return
	}
	if !bot.AuthorizedUsers[username] {
		log.Printf("[%s] unauthorized user", username)
		bot.replyToCommandWithMessage(update.Message, bot.Messages.Forbidden)
		return
	}

	err := bot.ChatDB.UpdateWith(username, update.Message.Chat.ID)
	if err != nil {
		log.Printf("[%s] cannot update chat db: %s", username, err)
	}

	if update.Message.ReplyToMessage != nil {
		// Only deal with forced replies (reply to bot's messages)
		if update.Message.ReplyToMessage.From == nil || update.Message.ReplyToMessage.From.UserName != bot.API.Self.UserName {
			return
		}

		if update.Message.ReplyToMessage.Text != "" {
			if update.Message.ReplyToMessage.Text == bot.Messages.MissingAlbumName {
				log.Printf("[%s] reply to previous command /%s: %s", username, bot.Commands.NewAlbum, text)
				bot.handleNewAlbumCommandReply(update.Message)
				return
			}
		}
	}

	if text != "" {
		if update.Message.IsCommand() {
			log.Printf("[%s] command: %s", username, text)
			switch update.Message.Command() {
			case "start", bot.Commands.Help:
				bot.handleHelpCommand(update.Message)
			case bot.Commands.Share:
				bot.handleShareCommand(update.Message)
			case bot.Commands.Browse:
				bot.handleBrowseCommand(update.Message)
			case bot.Commands.NewAlbum:
				bot.handleNewAlbumCommand(update.Message)
			case bot.Commands.Info:
				bot.handleInfoCommand(update.Message)
			default:
				bot.replyToCommandWithMessage(update.Message, bot.Messages.DoNotUnderstand)
			}
		} else {
			bot.replyToCommandWithMessage(update.Message, bot.Messages.DoNotUnderstand)
		}
	} else if update.Message.Photo != nil {
		err := bot.handlePhoto(update.Message)
		if err != nil {
			log.Printf("[%s] cannot add photo to current album: %s", username, err)
			bot.replyToCommandWithMessage(update.Message, bot.Messages.ServerError)
			return
		}
		bot.dispatchMessage(update.Message)
		bot.replyWithMessage(update.Message, bot.Messages.ThankYouMedia)
	} else if update.Message.Video != nil {
		err := bot.handleVideo(update.Message)
		if err != nil {
			log.Printf("[%s] cannot add video to current album: %s", username, err)
			bot.replyToCommandWithMessage(update.Message, bot.Messages.ServerError)
			return
		}
		bot.dispatchMessage(update.Message)
		bot.replyWithMessage(update.Message, bot.Messages.ThankYouMedia)
	} else {
		log.Printf("[%s] cannot handle this type of message", username)
		bot.replyToCommandWithMessage(update.Message, bot.Messages.DoNotUnderstand)
	}
}

func (bot *TelegramBot) dispatchMessage(message *tgbotapi.Message) {
	for user, _ := range bot.AuthorizedUsers {
		if user != message.From.UserName {
			if _, ok := bot.ChatDB.Db[user]; !ok {
				log.Printf("[%s] The chat db does not have any mapping for %s, skipping...", message.From.UserName, user)
				continue
			}

			msg := tgbotapi.NewForward(bot.ChatDB.Db[user], message.Chat.ID, message.MessageID)

			_, err := bot.API.Send(msg)
			if err != nil {
				log.Printf("[%s] Cannot dispatch message to %s (chat id = %d)", message.From.UserName, user, bot.ChatDB.Db[user])
			}
		}
	}
}

func (bot *TelegramBot) getFile(message *tgbotapi.Message, telegramFileId string, mediaStoreId string) error {
	url, err := bot.API.GetFileDirectURL(telegramFileId)
	if err != nil {
		return err
	}

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)

	n, err := resp.Body.Read(buffer)
	if err != nil {
		return err
	}

	// Detect the content-type
	contentType := http.DetectContentType(buffer)
	var extension string
	if contentType == "image/jpeg" {
		extension = ".jpeg"
	} else if contentType == "video/mp4" {
		extension = ".mp4"
	} else {
		log.Printf("[%s] Unknown media content-type '%s'", message.From.UserName, contentType)
		extension = ".bin"
	}

	// Create the file
	out, err := bot.MediaStore.AddFile(mediaStoreId + extension)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write back the first 512 bytes
	n, err = out.Write(buffer[0:n])
	if err != nil {
		return err
	}

	// Write the rest of the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func (bot *TelegramBot) handlePhoto(message *tgbotapi.Message) error {
	// Find the best resolution among all available sizes
	fileId := ""
	maxWidth := 0
	for _, photo := range *message.Photo {
		if photo.Width > maxWidth {
			fileId = photo.FileID
			maxWidth = photo.Width
		}
	}

	// Get a unique id
	mediaStoreId := bot.MediaStore.GetUniqueID()

	// Download the photo from the Telegram API and save it in the MediaStore
	err := bot.getFile(message, fileId, mediaStoreId)
	if err != nil {
		return err
	}

	// parse the message timestamp
	t := time.Unix(int64(message.Date), 0)
	return bot.MediaStore.CommitPhoto(mediaStoreId, t, message.Caption)
}
func (bot *TelegramBot) handleVideo(message *tgbotapi.Message) error {
	// Get a unique id
	mediaStoreId := bot.MediaStore.GetUniqueID()

	// Download the video from the Telegram API and save it in the MediaStore
	err := bot.getFile(message, message.Video.FileID, mediaStoreId)
	if err != nil {
		return err
	}

	// Download the video thumbnail from the Telegram API and save it in the MediaStore
	err = bot.getFile(message, message.Video.Thumbnail.FileID, mediaStoreId)
	if err != nil {
		log.Printf("[%s] Cannot download video thumbnail: %s", message.From.UserName, err)
	}

	// parse the message timestamp
	t := time.Unix(int64(message.Date), 0)
	return bot.MediaStore.CommitVideo(mediaStoreId, t, message.Caption)
}

func (bot *TelegramBot) handleHelpCommand(message *tgbotapi.Message) {
	bot.replyWithMessage(message, bot.Messages.Help)
}

func (bot *TelegramBot) handleShareCommand(message *tgbotapi.Message) {
	albumList, err := bot.MediaStore.ListAlbums()
	if err != nil {
		log.Printf("[%s] cannot get album list: %s", message.From.UserName, err)
		bot.replyToCommandWithMessage(message, bot.Messages.ServerError)
		return
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf(bot.Messages.SharedAlbum, bot.PerAlbumTokenValidity))
	text.WriteString("\n")
	sort.Sort(sort.Reverse(albumList))
	var tokenData TokenData = TokenData{
		Timestamp: time.Now(),
		Username:  message.From.UserName,
	}
	for _, album := range albumList {
		title := album.Title // TODO escape me
		id := album.ID
		if id == "" {
			id = "latest"
			title = title + " 🔥"
		}
		tokenData.Entitlement = id
		token := bot.TokenGenerator.NewToken(tokenData)
		url := fmt.Sprintf("%s/s/%s/%s/album/%s/", bot.WebPublicURL, url.PathEscape(message.From.UserName), url.PathEscape(token), url.PathEscape(id))
		text.WriteString(fmt.Sprintf("- [%s %s](%s)\n", album.Date.Format("2006-01"), title, url))
	}

	bot.replyWithMarkdownMessage(message, text.String())
}

func (bot *TelegramBot) handleBrowseCommand(message *tgbotapi.Message) {
	var tokenData TokenData = TokenData{
		Timestamp: time.Now(),
		Username:  message.From.UserName,
	}

	// Global share
	token := bot.TokenGenerator.NewToken(tokenData)
	url := fmt.Sprintf("%s/s/%s/%s/album/", bot.WebPublicURL, url.PathEscape(message.From.UserName), url.PathEscape(token))
	bot.replyWithMessage(message, fmt.Sprintf(bot.Messages.SharedGlobal, bot.GlobalTokenValidity))
	bot.replyWithMessage(message, url)
}

func (bot *TelegramBot) handleInfoCommand(message *tgbotapi.Message) {
	album, err := bot.MediaStore.GetCurrentAlbum()
	if err != nil {
		log.Printf("[%s] cannot get current album: %s", message.From.UserName, err)
		bot.replyToCommandWithMessage(message, bot.Messages.ServerError)
		return
	}

	if album.Title != "" {
		bot.replyWithMessage(message, fmt.Sprintf(bot.Messages.Info, album.Title))
	} else {
		bot.replyWithMessage(message, bot.Messages.InfoNoAlbum)
	}
}

func (bot *TelegramBot) handleNewAlbumCommand(message *tgbotapi.Message) {
	bot.replyWithForcedReply(message, bot.Messages.MissingAlbumName)
}

func (bot *TelegramBot) handleNewAlbumCommandReply(message *tgbotapi.Message) {
	albumName := message.Text

	err := bot.MediaStore.NewAlbum(albumName)
	if err != nil {
		log.Printf("[%s] cannot create album '%s': %s", message.From.UserName, albumName, err)
		bot.replyToCommandWithMessage(message, bot.Messages.ServerError)
		return
	}

	bot.replyWithMessage(message, bot.Messages.AlbumCreated)
}

func (telegram *TelegramBot) replyToCommandWithMessage(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID
	_, err := telegram.API.Send(msg)
	return err
}

func (telegram *TelegramBot) replyWithMessage(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	_, err := telegram.API.Send(msg)
	return err
}

func (telegram *TelegramBot) replyWithMarkdownMessage(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err := telegram.API.Send(msg)
	return err
}

func (telegram *TelegramBot) replyWithForcedReply(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply: true,
		Selective:  true,
	}
	_, err := telegram.API.Send(msg)
	return err
}
