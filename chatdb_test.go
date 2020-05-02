package main

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/magiconair/properties/assert"
)

func TestInitChatDB(t *testing.T) {
	tmp := createTempDir(t)
	defer tmp.cleanup(t)

	file := filepath.Join(tmp.RootDir, "chat.yaml")

	_, err := InitChatDB(file)
	if err != nil {
		t.Errorf("InitChatDB(): %s", err)
	}
	_, err = os.Stat(file)
	if err != nil {
		t.Errorf("InitChatDB(): chatdb not created (error = %s)", err)
	}
}

func TestUpdateWith(t *testing.T) {
	tmp := createTempDir(t)
	defer tmp.cleanup(t)

	file := filepath.Join(tmp.RootDir, "chat.yaml")

	chatdb, err := InitChatDB(file)
	if err != nil {
		t.Errorf("InitChatDB(): %s", err)
	}

	err = chatdb.UpdateWith("john", 123456)
	if err != nil {
		t.Errorf("UpdateWith(): %s", err)
	}

	if _, ok := chatdb.Db["john"]; !ok {
		t.Errorf("UpdateWith(): john is missing")
	}

	_, err = os.Stat(file + ".bak")
	if err != nil {
		t.Errorf("InitChatDB(): chatdb backup not created (error = %s)", err)
	}

	content, err := ioutil.ReadFile(file)
	if err != nil {
		t.Errorf("ioutil.ReadFile: %s", err)
	}
	assert.Equal(t, "john: 123456\n", string(content), "chatdb content")
}
