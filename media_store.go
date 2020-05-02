package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
	"gopkg.in/yaml.v2"
)

type MediaStore struct {
	StoreLocation string
}

func InitMediaStore(storeLocation string) (*MediaStore, error) {
	err := os.MkdirAll(filepath.Join(storeLocation, ".current"), os.ModePerm)
	if err != nil {
		return nil, err
	}
	return &MediaStore{StoreLocation: storeLocation}, nil
}

func (store *MediaStore) GetUniqueID() string {
	return uuid.New().String()
}

func (store *MediaStore) AddFile(fileName string) (*os.File, error) {
	filename := filepath.Join(store.StoreLocation, ".current", fileName)
	return os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0666)
}

func (store *MediaStore) CommitPhoto(id string, timestamp time.Time, caption string) error {
	return store.commitMedia(id, timestamp, caption, "photo")
}

func (store *MediaStore) CommitVideo(id string, timestamp time.Time, caption string) error {
	return store.commitMedia(id, timestamp, caption, "video")
}

func (store *MediaStore) commitMedia(id string, timestamp time.Time, caption string, mediaType string) error {
	entry := [1]map[string]string{{
		"type":    mediaType,
		"date":    timestamp.Format("2006-01-02T15:04:05-0700"),
		"caption": caption,
		"id":      id,
	}}

	yamlData, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}

	return appendToFile(filepath.Join(store.StoreLocation, ".current", "chat.yaml"), yamlData)
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

func (store *MediaStore) GetCurrentAlbum() (string, error) {
	yamlData, err := ioutil.ReadFile(filepath.Join(store.StoreLocation, ".current", "meta.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			// the album has not yet a name, it is not an error
			return "", nil
		} else {
			return "", err
		}
	}

	var metadata map[string]string = make(map[string]string)
	err = yaml.UnmarshalStrict(yamlData, &metadata)
	if err != nil {
		return "", err
	}

	return metadata["title"], nil
}

func (store *MediaStore) CloseAlbum() error {
	yamlData, err := ioutil.ReadFile(filepath.Join(store.StoreLocation, ".current", "meta.yaml"))
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
	err = os.Rename(filepath.Join(store.StoreLocation, ".current"), filepath.Join(store.StoreLocation, folderName))
	if err != nil {
		return err
	}

	return nil
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

func (store *MediaStore) NewAlbum(title string) error {
	if fileExists(filepath.Join(store.StoreLocation, ".current/")) {
		if fileExists(filepath.Join(store.StoreLocation, "/.current/meta.yaml")) {
			err := store.CloseAlbum()
			if err != nil {
				return err
			}
		}
	}

	err := os.MkdirAll(filepath.Join(store.StoreLocation, ".current/"), os.ModePerm)
	if err != nil {
		return err
	}

	metadata := map[string]string{
		"title": title,
		"date":  time.Now().Format("2006-01-02T15:04:05-0700"),
	}

	yamlData, err := yaml.Marshal(metadata)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(filepath.Join(store.StoreLocation, ".current", "meta.yaml"), yamlData, 0644)
	if err != nil {
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
