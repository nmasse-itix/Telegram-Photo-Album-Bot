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

type PhotoBot struct {
	Telegram     TelegramBackend
	MediaStore   *MediaStore
	WebInterface WebInterface
}

type TelegramBackend struct {
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

func InitBot(targetDir string) *PhotoBot {
	return &PhotoBot{
		Telegram: TelegramBackend{
			AuthorizedUsers: make(map[string]bool),
			RetryDelay:      time.Duration(30) * time.Second,
		},
	}
}

func (bot *PhotoBot) StartBot(token string) {
	var telegramBot *tgbotapi.BotAPI
	var err error

	for tryAgain := true; tryAgain; tryAgain = (err != nil) {
		telegramBot, err = tgbotapi.NewBotAPI(token)
		if err != nil {
			log.Printf("Cannot start the Telegram Bot because of '%s'. Retrying in %d seconds...", err, bot.Telegram.RetryDelay/time.Second)
			time.Sleep(bot.Telegram.RetryDelay)
		}
	}

	log.Printf("Authorized on account %s", telegramBot.Self.UserName)

	bot.Telegram.API = telegramBot
}

func (bot *PhotoBot) Process() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = bot.Telegram.NewUpdateTimeout
	updates, _ := bot.Telegram.API.GetUpdatesChan(u)
	for update := range updates {
		bot.ProcessUpdate(update)
	}
}

func (bot *PhotoBot) ProcessUpdate(update tgbotapi.Update) {
	if update.Message == nil || update.Message.From == nil {
		return
	}

	if update.Message.Chat == nil || update.Message.Chat.Type != "private" {
		return
	}

	text := update.Message.Text
	username := update.Message.From.UserName

	if username == "" {
		bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.NoUsername)
		return
	}
	if !bot.Telegram.AuthorizedUsers[username] {
		log.Printf("[%s] unauthorized user", username)
		bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.Forbidden)
		return
	}

	err := bot.Telegram.ChatDB.UpdateWith(username, update.Message.Chat.ID)
	if err != nil {
		log.Printf("[%s] cannot update chat db: %s", username, err)
	}

	if update.Message.ReplyToMessage != nil {
		// Only deal with forced replies (reply to bot's messages)
		if update.Message.ReplyToMessage.From == nil || update.Message.ReplyToMessage.From.UserName != bot.Telegram.API.Self.UserName {
			return
		}

		if update.Message.ReplyToMessage.Text != "" {
			if update.Message.ReplyToMessage.Text == bot.Telegram.Messages.MissingAlbumName {
				log.Printf("[%s] reply to previous command /%s: %s", username, bot.Telegram.Commands.NewAlbum, text)
				bot.handleNewAlbumCommandReply(update.Message)
				return
			}
		}
	}

	if text != "" {
		if update.Message.IsCommand() {
			log.Printf("[%s] command: %s", username, text)
			switch update.Message.Command() {
			case "start", bot.Telegram.Commands.Help:
				bot.handleHelpCommand(update.Message)
			case bot.Telegram.Commands.Share:
				bot.handleShareCommand(update.Message)
			case bot.Telegram.Commands.Browse:
				bot.handleBrowseCommand(update.Message)
			case bot.Telegram.Commands.NewAlbum:
				bot.handleNewAlbumCommand(update.Message)
			case bot.Telegram.Commands.Info:
				bot.handleInfoCommand(update.Message)
			default:
				bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.DoNotUnderstand)
			}
		} else {
			bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.DoNotUnderstand)
		}
	} else if update.Message.Photo != nil {
		err := bot.handlePhoto(update.Message)
		if err != nil {
			log.Printf("[%s] cannot add photo to current album: %s", username, err)
			bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.ServerError)
			return
		}
		bot.dispatchMessage(update.Message)
		bot.Telegram.replyWithMessage(update.Message, bot.Telegram.Messages.ThankYouMedia)
	} else if update.Message.Video != nil {
		err := bot.handleVideo(update.Message)
		if err != nil {
			log.Printf("[%s] cannot add video to current album: %s", username, err)
			bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.ServerError)
			return
		}
		bot.dispatchMessage(update.Message)
		bot.Telegram.replyWithMessage(update.Message, bot.Telegram.Messages.ThankYouMedia)
	} else {
		log.Printf("[%s] cannot handle this type of message", username)
		bot.Telegram.replyToCommandWithMessage(update.Message, bot.Telegram.Messages.DoNotUnderstand)
	}
}

func (bot *PhotoBot) dispatchMessage(message *tgbotapi.Message) {
	for user, _ := range bot.Telegram.AuthorizedUsers {
		if user != message.From.UserName {
			if _, ok := bot.Telegram.ChatDB.Db[user]; !ok {
				log.Printf("[%s] The chat db does not have any mapping for %s, skipping...", message.From.UserName, user)
				continue
			}

			msg := tgbotapi.NewForward(bot.Telegram.ChatDB.Db[user], message.Chat.ID, message.MessageID)

			_, err := bot.Telegram.API.Send(msg)
			if err != nil {
				log.Printf("[%s] Cannot dispatch message to %s (chat id = %d)", message.From.UserName, user, bot.Telegram.ChatDB.Db[user])
			}
		}
	}
}

func (bot *PhotoBot) getFile(message *tgbotapi.Message, telegramFileId string, mediaStoreId string) error {
	url, err := bot.Telegram.API.GetFileDirectURL(telegramFileId)
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

func (bot *PhotoBot) handlePhoto(message *tgbotapi.Message) error {
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
func (bot *PhotoBot) handleVideo(message *tgbotapi.Message) error {
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

func (bot *PhotoBot) handleHelpCommand(message *tgbotapi.Message) {
	bot.Telegram.replyWithMessage(message, bot.Telegram.Messages.Help)
}

func (bot *PhotoBot) handleShareCommand(message *tgbotapi.Message) {
	albumList, err := bot.MediaStore.ListAlbums()
	if err != nil {
		log.Printf("[%s] cannot get album list: %s", message.From.UserName, err)
		bot.Telegram.replyToCommandWithMessage(message, bot.Telegram.Messages.ServerError)
		return
	}

	var text strings.Builder
	text.WriteString(fmt.Sprintf(bot.Telegram.Messages.SharedAlbum, bot.Telegram.PerAlbumTokenValidity))
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
		token := bot.Telegram.TokenGenerator.NewToken(tokenData)
		url := fmt.Sprintf("%s/s/%s/%s/album/%s/", bot.Telegram.WebPublicURL, url.PathEscape(message.From.UserName), url.PathEscape(token), url.PathEscape(id))
		text.WriteString(fmt.Sprintf("- [%s %s](%s)\n", album.Date.Format("2006-01"), title, url))
	}

	bot.Telegram.replyWithMarkdownMessage(message, text.String())
}

func (bot *PhotoBot) handleBrowseCommand(message *tgbotapi.Message) {
	var tokenData TokenData = TokenData{
		Timestamp: time.Now(),
		Username:  message.From.UserName,
	}

	// Global share
	token := bot.Telegram.TokenGenerator.NewToken(tokenData)
	url := fmt.Sprintf("%s/s/%s/%s/album/", bot.Telegram.WebPublicURL, url.PathEscape(message.From.UserName), url.PathEscape(token))
	bot.Telegram.replyWithMessage(message, fmt.Sprintf(bot.Telegram.Messages.SharedGlobal, bot.Telegram.GlobalTokenValidity))
	bot.Telegram.replyWithMessage(message, url)
}

func (bot *PhotoBot) handleInfoCommand(message *tgbotapi.Message) {
	album, err := bot.MediaStore.GetCurrentAlbum()
	if err != nil {
		log.Printf("[%s] cannot get current album: %s", message.From.UserName, err)
		bot.Telegram.replyToCommandWithMessage(message, bot.Telegram.Messages.ServerError)
		return
	}

	if album.Title != "" {
		bot.Telegram.replyWithMessage(message, fmt.Sprintf(bot.Telegram.Messages.Info, album.Title))
	} else {
		bot.Telegram.replyWithMessage(message, bot.Telegram.Messages.InfoNoAlbum)
	}
}

func (bot *PhotoBot) handleNewAlbumCommand(message *tgbotapi.Message) {
	bot.Telegram.replyWithForcedReply(message, bot.Telegram.Messages.MissingAlbumName)
}

func (bot *PhotoBot) handleNewAlbumCommandReply(message *tgbotapi.Message) {
	albumName := message.Text

	err := bot.MediaStore.NewAlbum(albumName)
	if err != nil {
		log.Printf("[%s] cannot create album '%s': %s", message.From.UserName, albumName, err)
		bot.Telegram.replyToCommandWithMessage(message, bot.Telegram.Messages.ServerError)
		return
	}

	bot.Telegram.replyWithMessage(message, bot.Telegram.Messages.AlbumCreated)
}

func (telegram *TelegramBackend) replyToCommandWithMessage(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID
	_, err := telegram.API.Send(msg)
	return err
}

func (telegram *TelegramBackend) replyWithMessage(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	_, err := telegram.API.Send(msg)
	return err
}

func (telegram *TelegramBackend) replyWithMarkdownMessage(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ParseMode = tgbotapi.ModeMarkdown
	_, err := telegram.API.Send(msg)
	return err
}

func (telegram *TelegramBackend) replyWithForcedReply(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply: true,
		Selective:  true,
	}
	_, err := telegram.API.Send(msg)
	return err
}
