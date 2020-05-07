//go:generate statik -src=web/ -include=*.html,*.css,*.js,*.template

package main

import (
	"crypto"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

func initConfig() {
	// how many seconds to wait between retries, upon Telegram API errors
	viper.SetDefault("Telegram.RetryDelay", 60)
	// max duration between two telegram updates
	viper.SetDefault("Telegram.NewUpdateTimeout", 60)

	// Telegram messages
	viper.SetDefault("Telegram.Messages.Forbidden", "Access Denied")
	viper.SetDefault("Telegram.Messages.Help", `Hello, I'm the photo bot!

	You can send me your photos and videos.

	To start an album, use "/newAlbum".
	To get the current album name, use "/info".
	To share an album, use "/share album".
	To share all albums, use "/share".
	If you are lost, you can get this message again with "/help".

	Have a nice day!`)
	viper.SetDefault("Telegram.Messages.MissingAlbumName", "Which title should I give to the new album?")
	viper.SetDefault("Telegram.Messages.ServerError", "Server Internal Error")
	viper.SetDefault("Telegram.Messages.AlbumCreated", "Album created")
	viper.SetDefault("Telegram.Messages.DoNotUnderstand", "Sorry, I did not understand your request.")
	viper.SetDefault("Telegram.Messages.Info", "Current album is named %s. Please send me your photos and videos!")
	viper.SetDefault("Telegram.Messages.InfoNoAlbum", "There is no album started, yet.")
	viper.SetDefault("Telegram.Messages.NoUsername", "You need to set your Telegram username first!")
	viper.SetDefault("Telegram.Messages.ThankYouMedia", "Got it, thanks!")
	viper.SetDefault("Telegram.Messages.SharedAlbum", "Here are the albums and their sharing links. Links are valid for %d days.")
	viper.SetDefault("Telegram.Messages.SharedGlobal", "All albums can be reached with the following link. Link is valid for %d days.")

	// Telegram Commands
	viper.SetDefault("Telegram.Commands.Help", "help")
	viper.SetDefault("Telegram.Commands.Info", "info")
	viper.SetDefault("Telegram.Commands.NewAlbum", "newAlbum")
	viper.SetDefault("Telegram.Commands.Share", "share")
	viper.SetDefault("Telegram.Commands.Browse", "browse")

	// Web Interface
	viper.SetDefault("WebInterface.SiteName", "My photo album")
	viper.SetDefault("WebInterface.Listen", "127.0.0.1:8080")
	viper.SetDefault("WebInterface.Sessions.SecureCookie", true)
	viper.SetDefault("WebInterface.Sessions.CookieMaxAge", 86400*7)
	viper.SetDefault("Telegram.TokenGenerator.GlobalValidity", 7)
	viper.SetDefault("Telegram.TokenGenerator.PerAlbumValidity", 15)

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

	retryDelay := viper.GetInt("Telegram.RetryDelay")
	if retryDelay <= 0 {
		log.Fatal("The RetryDelay cannot be zero or negative!")
	}

	timeout := viper.GetInt("Telegram.NewUpdateTimeout")
	if timeout <= 0 {
		log.Fatal("The TelegramNewUpdateTimeout cannot be zero or negative!")
	}

	token := viper.GetString("Telegram.Token")
	if token == "" {
		log.Fatal("No Telegram Bot Token provided!")
	}

	authorizedUsersList := viper.GetStringSlice("Telegram.AuthorizedUsers")
	if len(authorizedUsersList) == 0 {
		log.Fatal("A list of AuthorizedUsers must be given!")
	}

	if viper.GetString("WebInterface.OIDC.DiscoveryUrl") == "" {
		log.Fatal("No OpenID Connect Discovery URL provided!")
	}

	if viper.GetString("WebInterface.OIDC.ClientID") == "" {
		log.Fatal("No OpenID Connect Client ID provided!")
	}

	if viper.GetString("WebInterface.OIDC.ClientSecret") == "" {
		log.Fatal("No OpenID Connect Client Secret provided!")
	}

	if viper.GetString("WebInterface.OIDC.RedirectURL") == "" {
		log.Fatal("No OpenID Connect Redirect URL provided!")
	}

	if viper.GetString("WebInterface.OIDC.ClientSecret") == "" {
		log.Fatal("No OpenID Connect Client Secret provided!")
	}

	if viper.GetString("WebInterface.Sessions.AuthenticationKey") == "" {
		log.Fatal("No Cookie Authentication Key provided!")
	}

	if viper.GetString("WebInterface.Sessions.EncryptionKey") == "" {
		log.Fatal("No Cookie Encryption Key provided!")
	}

	if viper.GetString("WebInterface.PublicURL") == "" {
		log.Fatal("No Public URL provided!")
	}

	if viper.GetString("Telegram.TokenGenerator.AuthenticationKey") == "" {
		log.Fatal("No Token Generator Authentication Key provided!")
	}
}

func getCommandsFromConfig() TelegramCommands {
	return TelegramCommands{
		Help:     viper.GetString("Telegram.Commands.Help"),
		NewAlbum: viper.GetString("Telegram.Commands.NewAlbum"),
		Info:     viper.GetString("Telegram.Commands.Info"),
		Share:    viper.GetString("Telegram.Commands.Share"),
		Browse:   viper.GetString("Telegram.Commands.Browse"),
	}
}

func getMessagesFromConfig() TelegramMessages {
	return TelegramMessages{
		Forbidden:        viper.GetString("Telegram.Messages.Forbidden"),
		Help:             viper.GetString("Telegram.Messages.Help"),
		MissingAlbumName: viper.GetString("Telegram.Messages.MissingAlbumName"),
		ServerError:      viper.GetString("Telegram.Messages.ServerError"),
		AlbumCreated:     viper.GetString("Telegram.Messages.AlbumCreated"),
		DoNotUnderstand:  viper.GetString("Telegram.Messages.DoNotUnderstand"),
		Info:             viper.GetString("Telegram.Messages.Info"),
		InfoNoAlbum:      viper.GetString("Telegram.Messages.InfoNoAlbum"),
		NoUsername:       viper.GetString("Telegram.Messages.NoUsername"),
		SharedAlbum:      viper.GetString("Telegram.Messages.SharedAlbum"),
		SharedGlobal:     viper.GetString("Telegram.Messages.SharedGlobal"),
	}
}

func main() {
	initConfig()
	validateConfig()

	// Create the Bot
	photoBot := InitBot(viper.GetString("TargetDir"))
	photoBot.Telegram.RetryDelay = time.Duration(viper.GetInt("Telegram.RetryDelay")) * time.Second
	photoBot.Telegram.NewUpdateTimeout = viper.GetInt("Telegram.NewUpdateTimeout")
	photoBot.Telegram.Commands = getCommandsFromConfig()
	photoBot.Telegram.Messages = getMessagesFromConfig()
	photoBot.WebInterface.SiteName = viper.GetString("WebInterface.SiteName")
	photoBot.Telegram.WebPublicURL = viper.GetString("WebInterface.PublicURL")

	// Fill the authorized users
	for _, item := range viper.GetStringSlice("Telegram.AuthorizedUsers") {
		photoBot.Telegram.AuthorizedUsers[item] = true
	}

	targetDir := viper.GetString("TargetDir")
	for _, dir := range []string{"data", "db"} {
		fullPath := filepath.Join(targetDir, dir)
		var err error = os.MkdirAll(fullPath, 0777)
		if err != nil {
			panic(fmt.Sprintf("os.MkdirAll: %s: %s\n", fullPath, err))
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
	photoBot.StartBot(viper.GetString("Telegram.Token"))
	photoBot.Telegram.API.Debug = viper.GetBool("Telegram.Debug")

	// Token Generator
	tokenAuthenticationKey, err := base64.StdEncoding.DecodeString(viper.GetString("Telegram.TokenGenerator.AuthenticationKey"))
	if err != nil {
		panic(err)
	}
	if len(tokenAuthenticationKey) < 32 {
		panic("The given token generator authentication key is too short!")
	}
	tokenGenerator, err := NewTokenGenerator(tokenAuthenticationKey, crypto.SHA256)
	if err != nil {
		panic(err)
	}
	photoBot.Telegram.TokenGenerator = tokenGenerator
	photoBot.Telegram.GlobalTokenValidity = viper.GetInt("Telegram.TokenGenerator.GlobalValidity")
	photoBot.Telegram.PerAlbumTokenValidity = viper.GetInt("Telegram.TokenGenerator.PerAlbumValidity")

	// Setup the web interface
	var oidc OpenIdSettings = OpenIdSettings{
		ClientID:     viper.GetString("WebInterface.OIDC.ClientID"),
		ClientSecret: viper.GetString("WebInterface.OIDC.ClientSecret"),
		DiscoveryUrl: viper.GetString("WebInterface.OIDC.DiscoveryUrl"),
		RedirectURL:  viper.GetString("WebInterface.OIDC.RedirectURL"),
		GSuiteDomain: viper.GetString("WebInterface.OIDC.GSuiteDomain"),
		Scopes:       viper.GetStringSlice("WebInterface.OIDC.Scopes"),
	}
	authenticationKey, err := base64.StdEncoding.DecodeString(viper.GetString("WebInterface.Sessions.AuthenticationKey"))
	if err != nil {
		panic(err)
	}
	if len(authenticationKey) < 32 {
		panic("The given session authentication key is too short!")
	}
	encryptionKey, err := base64.StdEncoding.DecodeString(viper.GetString("WebInterface.Sessions.EncryptionKey"))
	if err != nil {
		panic(err)
	}
	if len(encryptionKey) < 32 {
		panic("The given session encryption key is too short!")
	}
	var sessions SessionSettings = SessionSettings{
		AuthenticationKey: authenticationKey,
		EncryptionKey:     encryptionKey,
		CookieMaxAge:      viper.GetInt("WebInterface.Sessions.CookieMaxAge"),
		SecureCookie:      viper.GetBool("WebInterface.Sessions.SecureCookie"),
	}
	securityFrontend, err := NewSecurityFrontend(oidc, sessions, tokenGenerator)
	if err != nil {
		panic(err)
	}
	securityFrontend.GlobalTokenValidity = viper.GetInt("Telegram.TokenGenerator.GlobalValidity")
	securityFrontend.PerAlbumTokenValidity = viper.GetInt("Telegram.TokenGenerator.PerAlbumValidity")

	initLogFile()
	go photoBot.Process()
	photoBot.ServeWebInterface(viper.GetString("WebInterface.Listen"), securityFrontend)
}
