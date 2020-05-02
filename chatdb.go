package main

import (
	"io/ioutil"
	"log"
	"os"

	"gopkg.in/yaml.v2"
)

type ChatDB struct {
	Path string

	// Map usernames to chat id
	Db map[string]int64
}

func InitChatDB(path string) (*ChatDB, error) {
	db := make(map[string]int64)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	yamlData, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(yamlData, &db)
	if err != nil {
		return nil, err
	}

	return &ChatDB{Path: path, Db: db}, nil
}

func (chatdb *ChatDB) UpdateWith(username string, chatId int64) error {
	if _, ok := chatdb.Db[username]; !ok {
		chatdb.Db[username] = chatId

		yamlData, err := yaml.Marshal(chatdb.Db)
		if err != nil {
			return err
		}

		err = os.Rename(chatdb.Path, chatdb.Path+".bak")
		if err != nil {
			log.Printf("Cannot perform a backup of the chatdb before update: %s", err)
		}

		err = ioutil.WriteFile(chatdb.Path, yamlData, 0600)
		if err != nil {
			return err
		}
	}

	return nil
}
