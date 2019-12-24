package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/spf13/viper"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v2"
)

var chatDB map[string]int64 = make(map[string]int64)

func main() {
	viper.SetDefault("MsgForbidden", "Access Denied")
	viper.SetDefault("MsgHelp", "Hello")
	viper.SetDefault("MsgMissingAlbumName", "The album name is missing")
	viper.SetDefault("MsgAlbumAlreadyCreated", "An album has already been created")
	viper.SetDefault("MsgServerError", "Server Error")
	viper.SetDefault("MsgAlbumCreated", "Album created")
	viper.SetDefault("MsgNoAlbum", "No album is currently open")
	viper.SetDefault("MsgAlbumClosed", "Album closed")
	viper.SetDefault("MsgDoNotUnderstand", "Unknown command")
	viper.SetDefault("MsgNoUsername", "Sorry, you need to set your username")
	viper.SetDefault("MsgThankYouMedia", "Got it, thanks!")
	viper.SetDefault("MsgThankYouText", "Thank you!")
	viper.SetDefault("MsgSendMeSomething", "OK. Send me something.")

	viper.SetConfigName("photo-bot") // name of config file (without extension)
	viper.AddConfigPath("/etc/photo-bot/")
	viper.AddConfigPath("$HOME/.photo-bot")
	viper.AddConfigPath(".") // optionally look for config in the working directory
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Cannot read config file: %s\n", err))
	}

	logFile := viper.GetString("LogFile")
	var logHandle *os.File
	if logFile != "" {
		logHandle, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			panic(fmt.Errorf("Cannot open log file '%s': %s\n", logFile, err))
		}
		defer logHandle.Close()
		log.SetOutput(logHandle)
	}

	target_dir := viper.GetString("TargetDir")
	if target_dir == "" {
		panic("No target directory provided!")
	}
	_, err = os.Stat(target_dir)
	if err != nil && os.IsNotExist(err) {
		panic(fmt.Errorf("Cannot find target directory: %s: %s\n", target_dir, err))
	}

	authorized_users_list := viper.GetStringSlice("AuthorizedUsers")
	if len(authorized_users_list) == 0 {
		panic(fmt.Errorf("A list of AuthorizedUsers must be given\n"))
	}
	authorized_users := map[string]bool{}
	for _, item := range authorized_users_list {
		authorized_users[item] = true
	}

	token := viper.GetString("TelegramToken")
	if token == "" {
		panic("No Telegram Bot Token provided!")
	}

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	bot.Debug = viper.GetBool("TelegramDebug")

	log.Printf("Authorized on account %s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates, err := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil || update.Message.From == nil {
			continue
		}

		text := update.Message.Text
		username := update.Message.From.UserName

		if username == "" {
			replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgNoUsername"))
			continue
		}
		if !authorized_users[username] {
			log.Printf("[%s] unauthorized user", username)
			replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgForbidden"))
			continue
		}

		err := updateChatDB(update.Message)
		if err != nil {
			log.Printf("[%s] cannot update chat db: %s", username, err)
		}

		if text != "" {
			if update.Message.IsCommand() {
				log.Printf("[%s] command: %s", username, text)
				switch update.Message.Command() {
				case "start", "aide", "help":
					replyWithMessage(bot, update.Message, viper.GetString("MsgHelp"))
				case "nouvelAlbum":
					if len(text) < 14 {
						replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgMissingAlbumName"))
						continue
					}
					albumName := update.Message.CommandArguments()

					if albumAlreadyOpen() {
						replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgAlbumAlreadyCreated"))
						continue
					}

					err := newAlbum(update.Message, albumName)
					if err != nil {
						log.Printf("[%s] cannot create album '%s': %s", username, albumName, err)
						replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgServerError"))
						continue
					}

					replyWithMessage(bot, update.Message, viper.GetString("MsgAlbumCreated"))
				case "info":
					if albumAlreadyOpen() {
						albumName, err := getInfo()
						if err != nil {
							log.Printf("[%s] cannot close current album: %s", username, err)
							replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgServerError"))
							continue
						}
						replyWithMessage(bot, update.Message, fmt.Sprintf(viper.GetString("MsgInfo"), albumName))
					} else {
						replyWithMessage(bot, update.Message, viper.GetString("MsgInfoNoAlbum"))
					}
				case "pourLouise":
					if !albumAlreadyOpen() {
						replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgNoAlbum"))
						continue
					}
					replyWithForcedReply(bot, update.Message, viper.GetString("MsgSendMeSomething"))
				case "cloreAlbum":
					if !albumAlreadyOpen() {
						replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgNoAlbum"))
						continue
					}

					err := closeAlbum()
					if err != nil {
						log.Printf("[%s] cannot close current album: %s", username, err)
						replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgServerError"))
						continue
					}

					replyWithMessage(bot, update.Message, viper.GetString("MsgAlbumClosed"))
				default:
					replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgDoNotUnderstand"))
					continue
				}
			} else {
				if !albumAlreadyOpen() {
					replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgNoAlbum"))
					continue
				}

				err := addMessageToAlbum(update.Message)
				if err != nil {
					log.Printf("[%s] cannot add text '%s' to current album: %s", username, update.Message.Text, err)
					replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgServerError"))
					continue
				}
				replyWithMessage(bot, update.Message, viper.GetString("MsgThankYouText"))
			}
		} else if update.Message.Photo != nil {
			if !albumAlreadyOpen() {
				replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgNoAlbum"))
				continue
			}

			err := handlePhoto(bot, update.Message)
			if err != nil {
				log.Printf("[%s] cannot add photo to current album: %s", username, err)
				replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgServerError"))
				continue
			}
			dispatchMessage(bot, update.Message)
			replyWithMessage(bot, update.Message, viper.GetString("MsgThankYouMedia"))
		} else if update.Message.Video != nil {
			if !albumAlreadyOpen() {
				replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgNoAlbum"))
				continue
			}

			err := handleVideo(bot, update.Message)
			if err != nil {
				log.Printf("[%s] cannot add video to current album: %s", username, err)
				replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgServerError"))
				continue
			}
			dispatchMessage(bot, update.Message)
			replyWithMessage(bot, update.Message, viper.GetString("MsgThankYouMedia"))
		} else {
			log.Printf("[%s] cannot handle this type of message", username)
			replyToCommandWithMessage(bot, update.Message, viper.GetString("MsgDoNotUnderstand"))
			continue
		}
	}
}

func updateChatDB(message *tgbotapi.Message) error {
	target_dir := viper.GetString("TargetDir")
	if len(chatDB) == 0 {
		yamlData, err := ioutil.ReadFile(target_dir + "/db/chatdb.yaml")
		if err != nil {
			log.Printf("cannot read chat db: %s", err)
		} else {
			err = yaml.Unmarshal(yamlData, &chatDB)
			if err != nil {
				log.Printf("cannot unmarshal chat db: %s", err)
			}
		}
	}

	if _, ok := chatDB[message.From.UserName]; !ok {
		chatDB[message.From.UserName] = message.Chat.ID

		yamlData, err := yaml.Marshal(chatDB)
		if err != nil {
			return err
		}

		os.MkdirAll(target_dir+"/db/", os.ModePerm)
		err = ioutil.WriteFile(target_dir+"/db/chatdb.yaml", yamlData, 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

func replyWithForcedReply(bot *tgbotapi.BotAPI, message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyMarkup = tgbotapi.ForceReply{
		ForceReply: true,
		Selective:  true,
	}
	_, err := bot.Send(msg)
	return err
}

func replyToCommandWithMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	msg.ReplyToMessageID = message.MessageID
	_, err := bot.Send(msg)
	return err
}

func replyWithMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message, text string) error {
	msg := tgbotapi.NewMessage(message.Chat.ID, text)
	_, err := bot.Send(msg)
	return err
}

func dispatchMessage(bot *tgbotapi.BotAPI, message *tgbotapi.Message) {
	users := viper.GetStringSlice("AuthorizedUsers")
	for _, user := range users {
		if user != message.From.UserName {
			if _, ok := chatDB[user]; !ok {
				log.Printf("[%s] The chat db does not have any mapping for %s, skipping...", message.From.UserName, user)
				continue
			}

			msg := tgbotapi.NewForward(chatDB[user], message.Chat.ID, message.MessageID)

			_, err := bot.Send(msg)
			if err != nil {
				log.Printf("[%s] Cannot dispatch message to %s (chat id = %d)", message.From.UserName, user, chatDB[user])
			}
		}
	}
}

func handlePhoto(bot *tgbotapi.BotAPI, message *tgbotapi.Message) error {
	fileId := ""
	maxWidth := 0
	for _, photo := range *message.Photo {
		if photo.Width > maxWidth {
			fileId = photo.FileID
			maxWidth = photo.Width
		}
	}

	photoFileName, err := getFile(bot, message, fileId)
	if err != nil {
		return err
	}

	// parse the message timestamp
	t := time.Unix(int64(message.Date), 0)
	chat := [1]map[string]string{{
		"type":           "photo",
		"date":           t.Format("2006-01-02T15:04:05-0700"),
		"from":           "telegram",
		"telegramFileId": fileId,
		"username":       message.From.UserName,
		"firstname":      message.From.FirstName,
		"lastname":       message.From.LastName,
		"filename":       photoFileName,
	}}

	yamlData, err := yaml.Marshal(chat)
	if err != nil {
		return err
	}

	target_dir := viper.GetString("TargetDir")
	return appendToFile(target_dir+"/data/.current/chat.yaml", yamlData)
}

func handleVideo(bot *tgbotapi.BotAPI, message *tgbotapi.Message) error {
	videoFileName, err := getFile(bot, message, message.Video.FileID)
	if err != nil {
		return err
	}

	thumbFileName, err := getFile(bot, message, message.Video.Thumbnail.FileID)
	if err != nil {
		log.Printf("[%s] Cannot download video thumbnail: %s", message.From.UserName, err)
	}

	// parse the message timestamp
	t := time.Unix(int64(message.Date), 0)
	chat := [1]map[string]string{{
		"type":           "video",
		"date":           t.Format("2006-01-02T15:04:05-0700"),
		"from":           "telegram",
		"telegramFileId": message.Video.FileID,
		"username":       message.From.UserName,
		"firstname":      message.From.FirstName,
		"lastname":       message.From.LastName,
		"filename":       videoFileName,
		"thumb_filename": thumbFileName,
	}}

	yamlData, err := yaml.Marshal(chat)
	if err != nil {
		return err
	}

	target_dir := viper.GetString("TargetDir")
	return appendToFile(target_dir+"/data/.current/chat.yaml", yamlData)
}

func getFile(bot *tgbotapi.BotAPI, message *tgbotapi.Message, fileId string) (string, error) {
	url, err := bot.GetFileDirectURL(fileId)
	if err != nil {
		return "", err
	}

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Only the first 512 bytes are used to sniff the content type.
	buffer := make([]byte, 512)

	n, err := resp.Body.Read(buffer)
	if err != nil {
		return "", err
	}

	// Detect the content-type
	contentType := http.DetectContentType(buffer)
	extension := ".bin"
	if contentType == "image/jpeg" {
		extension = ".jpeg"
	} else if contentType == "video/mp4" {
		extension = ".mp4"
	} else {
		log.Printf("[%s] Unknown media content-type '%s'", message.From.UserName, contentType)
	}

	// Create the file
	target_dir := viper.GetString("TargetDir")
	filename := target_dir + "/data/.current/" + fileId + extension
	out, err := os.Create(filename)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Write back the first 512 bytes
	n, err = out.Write(buffer[0:n])
	if err != nil {
		return "", err
	}

	// Write the rest of the body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	return fileId + extension, nil
}

func closeAlbum() error {
	target_dir := viper.GetString("TargetDir")
	yamlData, err := ioutil.ReadFile(target_dir + "/data/.current/meta.yaml")
	if err != nil {
		return err
	}

	var metadata map[string]string = make(map[string]string)
	err = yaml.UnmarshalStrict(yamlData, &metadata)
	if err != nil {
		return err
	}

	date, err := time.Parse("2006-01-02T15:04:05-0700", metadata["date"])
	if err != nil {
		return err
	}

	folderName := date.Format("2006-01-02") + "-" + sanitizeAlbumName(metadata["title"])
	err = os.Rename(target_dir+"/data/.current/", target_dir+"/data/"+folderName)
	if err != nil {
		return err
	}

	return nil
}

func getInfo() (string, error) {
	target_dir := viper.GetString("TargetDir")
	yamlData, err := ioutil.ReadFile(target_dir + "/data/.current/meta.yaml")
	if err != nil {
		return "", err
	}

	var metadata map[string]string = make(map[string]string)
	err = yaml.UnmarshalStrict(yamlData, &metadata)
	if err != nil {
		return "", err
	}

	return metadata["title"], nil
}

func albumAlreadyOpen() bool {
	target_dir := viper.GetString("TargetDir")
	_, err := os.Stat(target_dir + "/data/.current")
	return err == nil
}

func newAlbum(message *tgbotapi.Message, albumName string) error {
	target_dir := viper.GetString("TargetDir")
	os.MkdirAll(target_dir+"/data/.current", os.ModePerm)

	metadata := map[string]string{
		"title":     albumName,
		"from":      "telegram",
		"username":  message.From.UserName,
		"firstname": message.From.FirstName,
		"lastname":  message.From.LastName,
		"date":      time.Now().Format("2006-01-02T15:04:05-0700"),
	}

	yamlData, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(target_dir+"/data/.current/meta.yaml", yamlData, 0644)
	if err != nil {
		return err
	}

	return nil
}

func addMessageToAlbum(message *tgbotapi.Message) error {
	target_dir := viper.GetString("TargetDir")

	// parse the message timestamp
	t := time.Unix(int64(message.Date), 0)
	chat := [1]map[string]string{{
		"type":      "text",
		"date":      t.Format("2006-01-02T15:04:05-0700"),
		"from":      "telegram",
		"username":  message.From.UserName,
		"firstname": message.From.FirstName,
		"lastname":  message.From.LastName,
		"message":   message.Text,
	}}

	yamlData, err := yaml.Marshal(chat)
	if err != nil {
		return err
	}

	return appendToFile(target_dir+"/data/.current/chat.yaml", yamlData)
}

func appendToFile(filename string, data []byte) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	return nil
}

func sanitizeAlbumName(albumName string) string {
	albumName = strings.ToLower(albumName)
	t := transform.Chain(norm.NFD, transform.RemoveFunc(func(r rune) bool {
		return unicode.Is(unicode.Mn, r)
	}), norm.NFC)
	albumName, _, _ = transform.String(t, albumName)

	reg, err := regexp.Compile("\\s+")
	if err != nil {
		panic(fmt.Errorf("Cannot compile regex: %s", err))
	}
	albumName = reg.ReplaceAllString(albumName, "-")

	reg, err = regexp.Compile("[^-a-zA-Z0-9_]+")
	if err != nil {
		panic(fmt.Errorf("Cannot compile regex: %s", err))
	}
	albumName = reg.ReplaceAllString(albumName, "")

	return albumName
}
