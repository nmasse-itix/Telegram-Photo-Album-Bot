package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

func initConfig() {
	// how many seconds to wait between retries, upon Telegram API errors
	viper.SetDefault("RetryDelay", 60)
	// max duration between two telegram updates
	viper.SetDefault("TelegramNewUpdateTimeout", 60)

	// Default messages
	viper.SetDefault("MsgForbidden", "Access Denied")
	viper.SetDefault("MsgHelp", "Hello")
	viper.SetDefault("MsgMissingAlbumName", "The album name is missing")
	viper.SetDefault("MsgServerError", "Server Error")
	viper.SetDefault("MsgAlbumCreated", "Album created")
	viper.SetDefault("MsgDoNotUnderstand", "Unknown command")
	viper.SetDefault("MsgInfoNoAlbum", "There is no album started, yet.")
	viper.SetDefault("MsgNoUsername", "Sorry, you need to set your username")
	viper.SetDefault("MsgThankYouMedia", "Got it, thanks!")
	viper.SetDefault("MsgSendMeSomething", "OK. Send me something.")

	viper.SetConfigName("photo-bot") // name of config file (without extension)
	viper.AddConfigPath("/etc/photo-bot/")
	viper.AddConfigPath("$HOME/.photo-bot")
	viper.AddConfigPath(".") // optionally look for config in the working directory
	err := viper.ReadInConfig()
	if err != nil {
		panic(fmt.Errorf("Cannot read config file: %s\n", err))
	}
}

func initLogFile() {
	logFile := viper.GetString("LogFile")
	if logFile != "" {
		logHandle, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if err != nil {
			panic(fmt.Errorf("Cannot open log file '%s': %s\n", logFile, err))
		}
		log.SetOutput(logHandle)
	}
}

func validateConfig() {
	targetDir := viper.GetString("TargetDir")
	if targetDir == "" {
		log.Fatal("No target directory provided!")
	}
	_, err := os.Stat(targetDir)
	if err != nil && os.IsNotExist(err) {
		log.Fatalf("Cannot find target directory: %s: %s", targetDir, err)
	}

	retryDelay := viper.GetInt("RetryDelay")
	if retryDelay <= 0 {
		log.Fatal("The TelegramNewUpdateTimeout cannot be zero or negative!")
	}

	timeout := viper.GetInt("TelegramNewUpdateTimeout")
	if timeout <= 0 {
		log.Fatal("The TelegramNewUpdateTimeout cannot be zero or negative!")
	}

	token := viper.GetString("TelegramToken")
	if token == "" {
		log.Fatal("No Telegram Bot Token provided!")
	}

	authorizedUsersList := viper.GetStringSlice("AuthorizedUsers")
	if len(authorizedUsersList) == 0 {
		log.Fatal("A list of AuthorizedUsers must be given!")
	}
}

func main() {
	initConfig()
	validateConfig()

	// Create the Bot
	photoBot := InitBot(viper.GetString("TargetDir"))
	photoBot.Telegram.RetryDelay = time.Duration(viper.GetInt("RetryDelay")) * time.Second
	photoBot.Telegram.NewUpdateTimeout = viper.GetInt("TelegramNewUpdateTimeout")

	// Fill the authorized users
	for _, item := range viper.GetStringSlice("AuthorizedUsers") {
		photoBot.Telegram.AuthorizedUsers[item] = true
	}

	targetDir := viper.GetString("TargetDir")
	for _, dir := range []string{"data", "db"} {
		fullPath := filepath.Join(targetDir, dir)
		var err error = os.MkdirAll(fullPath, 0777)
		if err != nil {
			log.Fatalf("os.MkdirAll: %s: %s\n", fullPath, err)
		}
	}

	// Create the ChatDB and inject it
	chatDB, err := InitChatDB(filepath.Join(targetDir, "db", "chatdb.yaml"))
	if err != nil {
		panic(err)
	}
	photoBot.Telegram.ChatDB = chatDB

	// Create the MediaStore and inject it
	mediaStore, err := InitMediaStore(filepath.Join(targetDir, "data"))
	if err != nil {
		panic(err)
	}
	photoBot.MediaStore = mediaStore

	// Start the bot
	photoBot.StartBot(viper.GetString("TelegramToken"))
	photoBot.Telegram.API.Debug = viper.GetBool("TelegramDebug")

	initLogFile()
	photoBot.Process()
}
