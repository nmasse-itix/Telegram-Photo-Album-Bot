package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/spf13/viper"
)

type PhotoBot struct {
	Telegram   TelegramBackend
	MediaStore *MediaStore
}

type TelegramBackend struct {
	ChatDB           *ChatDB
	AuthorizedUsers  map[string]bool
	RetryDelay       time.Duration
	NewUpdateTimeout int
	API              *tgbotapi.BotAPI
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

	text := update.Message.Text
	username := update.Message.From.UserName

	if username == "" {
		bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgNoUsername"))
		return
	}
	if !bot.Telegram.AuthorizedUsers[username] {
		log.Printf("[%s] unauthorized user", username)
		bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgForbidden"))
		return
	}

	err := bot.Telegram.ChatDB.UpdateWith(username, update.Message.Chat.ID)
	if err != nil {
		log.Printf("[%s] cannot update chat db: %s", username, err)
	}

	if text != "" {
		if update.Message.IsCommand() {
			log.Printf("[%s] command: %s", username, text)
			switch update.Message.Command() {
			case "start", "aide", "help":
				bot.handleHelpCommand(update.Message)
			case "nouvelAlbum":
				bot.handleNewAlbumCommand(update.Message)
			case "info":
				bot.handleInfoCommand(update.Message)
			case "pourLouise":
				bot.Telegram.replyWithForcedReply(update.Message, viper.GetString("MsgSendMeSomething"))
			default:
				bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgDoNotUnderstand"))
			}
		} else {
			bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgDoNotUnderstand"))
		}
	} else if update.Message.Photo != nil {
		err := bot.handlePhoto(update.Message)
		if err != nil {
			log.Printf("[%s] cannot add photo to current album: %s", username, err)
			bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgServerError"))
			return
		}
		bot.dispatchMessage(update.Message)
		bot.Telegram.replyWithMessage(update.Message, viper.GetString("MsgThankYouMedia"))
	} else if update.Message.Video != nil {
		err := bot.handleVideo(update.Message)
		if err != nil {
			log.Printf("[%s] cannot add video to current album: %s", username, err)
			bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgServerError"))
			return
		}
		bot.dispatchMessage(update.Message)
		bot.Telegram.replyWithMessage(update.Message, viper.GetString("MsgThankYouMedia"))
	} else {
		log.Printf("[%s] cannot handle this type of message", username)
		bot.Telegram.replyToCommandWithMessage(update.Message, viper.GetString("MsgDoNotUnderstand"))
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
	bot.Telegram.replyWithMessage(message, viper.GetString("MsgHelp"))
}

func (bot *PhotoBot) handleInfoCommand(message *tgbotapi.Message) {
	albumName, err := bot.MediaStore.GetCurrentAlbum()
	if err != nil {
		log.Printf("[%s] cannot get current album: %s", message.From.UserName, err)
		bot.Telegram.replyToCommandWithMessage(message, viper.GetString("MsgServerError"))
		return
	}

	if albumName != "" {
		bot.Telegram.replyWithMessage(message, fmt.Sprintf(viper.GetString("MsgInfo"), albumName))
	} else {
		bot.Telegram.replyWithMessage(message, viper.GetString("MsgInfoNoAlbum"))
	}
}

func (bot *PhotoBot) handleNewAlbumCommand(message *tgbotapi.Message) {
	if len(message.Text) < 14 {
		bot.Telegram.replyToCommandWithMessage(message, viper.GetString("MsgMissingAlbumName"))
		return
	}
	albumName := message.CommandArguments()

	err := bot.MediaStore.NewAlbum(albumName)
	if err != nil {
		log.Printf("[%s] cannot create album '%s': %s", message.From.UserName, albumName, err)
		bot.Telegram.replyToCommandWithMessage(message, viper.GetString("MsgServerError"))
		return
	}

	bot.Telegram.replyWithMessage(message, viper.GetString("MsgAlbumCreated"))
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

func (telegram *TelegramBackend) replyWithForcedReply(message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply: true,
		Selective:  true,
	}
	_, err := telegram.API.Send(msg)
	return err
}
