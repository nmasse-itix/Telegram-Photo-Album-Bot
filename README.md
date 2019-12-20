# The Photo-Album Bot for Telegram

## Compilation

Fetch dependencies.

```sh
go get -u github.com/go-telegram-bot-api/telegram-bot-api
go get -u github.com/spf13/viper
go get -u gopkg.in/yaml.v2
```

Compile for Raspberry PI.

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

Create a file named `photo-bot.yaml` in `/opt/photo-bot/etc/`.

```yaml
TelegramToken: "bot.token.here"
TelegramDebug: true
TargetDir: /srv/photo-bot
AuthorizedUsers:
- john
- jane
```

```sh
chown bot:bot /opt/photo-bot/etc/photo-bot.yaml
chmod 600 /opt/photo-bot/etc/photo-bot.yaml
```

Start the bot manually.

```sh
sudo -u bot /opt/photo-bot/bin/photo-bot
```

Create the startup script in `/etc/init.d/photo-bot`.

```sh
#!/bin/sh /etc/rc.common
# photo-bot

# Start late in the boot process
START=80
STOP=20

start() {
  cd /opt/photo-bot/etc/ && start-stop-daemon -c bot -u bot -x /opt/photo-bot/bin/photo-bot -b -S
}

stop() {
  start-stop-daemon -c bot -u bot -x /opt/photo-bot/bin/photo-bot -b -K
}
```

```sh
chmod 755 /etc/init.d/photo-bot
service photo-bot enable
service photo-bot start
```



## Documentation

- https://core.telegram.org/bots/api
