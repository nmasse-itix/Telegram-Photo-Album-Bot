# The Photo-Album Bot for Telegram

## Compilation

Pack all web files

```sh
go generate
```

Compile for your target platform (example given for a Raspberry PI 3).

```sh
GOOS=linux GOARCH=arm64 go build -o photo-bot
```

## Create a Bot

Talk to [BotFather](https://core.telegram.org/bots#6-botfather) to create your bot.

```
/newbot
```

Keep your bot token secure and safe!

## Installation

On your Raspberry PI.

```sh
mkdir -p /opt/photo-bot/bin
mkdir -p /opt/photo-bot/etc
mkdir -p /srv/photo-bot
useradd -d /srv/photo-bot -s /bin/false -m -r bot
chown bot:bot /srv/photo-bot
```

```sh
scp photo-bot root@raspberry-pi.example.test:/opt/photo-bot/bin/
```

Create a file named `photo-bot.yaml` in `/opt/photo-bot/etc/`, using the [provided config sample](configs/photo-bot.yaml) as a starting base.

**Note:** the Authentication and Encryption Keys can be created using `openssl rand -base64 32`

```sh
chown bot:bot /opt/photo-bot/etc/photo-bot.yaml
chmod 600 /opt/photo-bot/etc/photo-bot.yaml
```

Start the bot manually.

```sh
sudo -u bot /opt/photo-bot/bin/photo-bot
```

Create the startup script in `/etc/init.d/photo-bot`.
A sample init script is [provided in the init folder](init/photo-bot).

```sh
chmod 755 /etc/init.d/photo-bot
service photo-bot enable
service photo-bot start
```

## Useful notes

Video autoplay is tricky:

- On Firefox, you have to interact with the page first (click somewhere in the page)
- On Safari, you have to [explicitly enable auto-play for this website](https://support.apple.com/fr-fr/guide/safari/ibrw29c6ecf8/mac)
- On Chrome, it seems to be enabled out-of-the-box

## Documentation

- https://core.telegram.org/bots/api
- https://go-telegram-bot-api.github.io
- https://github.com/go-telegram-bot-api/telegram-bot-api
