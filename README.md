# The Photo-Album Bot for Telegram

## Compilation

```
go get -u github.com/go-telegram-bot-api/telegram-bot-api
go get -u github.com/spf13/viper
go get -u gopkg.in/yaml.v2
```

## Create a Bot

Talk to [BotFather](https://core.telegram.org/bots#6-botfather) to create your bot.

```
/newbot
```

Keep your bot token secure and safe!

## Create the configuration file

Create a file named `photo-bot.yaml` in the current directory.

```yaml
TelegramToken: "bot.token.here"
TelegramDebug: true
TargetDir: /srv/photos
AuthorizedUsers:
- john
- jane
```

## Documentation

- https://core.telegram.org/bots/api
