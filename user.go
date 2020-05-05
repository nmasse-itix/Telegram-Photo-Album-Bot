package main

import "fmt"

type UserType int

const (
	TypeAnonymous    UserType = 0
	TypeTelegramUser UserType = 1
	TypeOidcUser     UserType = 2
)

func (t UserType) String() string {
	names := [...]string{
		"Anonymous",
		"Telegram",
		"OIDC",
	}

	if t < TypeAnonymous || t > TypeOidcUser {
		return "Unknown"
	}

	return names[t]
}

type WebUser struct {
	Username string
	Type     UserType
}

func (u WebUser) String() string {
	if u.Type == TypeAnonymous {
		return "Anonymous"
	}

	return fmt.Sprintf("%s:%s", u.Type, u.Username)
}
