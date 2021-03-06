package main

import (
	"fmt"
	"io/ioutil"
	"log"
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

type Album struct {
	ID         string    `yaml:"-"` // Not part of the YAML struct
	Title      string    `yaml:"title"`
	Date       time.Time `yaml:"date"`
	Media      []Media   `yaml:"-"` // Not part of the YAML struct
	CoverMedia Media     `yaml:"cover,omitempty"`
}

type Media struct {
	Type    string    `yaml:"type"`
	ID      string    `yaml:"id"`
	Files   []string  `yaml:"-"` // Not part of the YAML struct
	Caption string    `yaml:"caption"`
	Date    time.Time `yaml:"date"`
}

// A media without ID will not be serialized in YAML
func (m *Media) IsZero() bool {
	return m.ID == ""
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
	entry := [1]Media{{
		Type:    mediaType,
		Date:    timestamp,
		Caption: caption,
		ID:      id,
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

type AlbumList []Album

func (list AlbumList) Len() int {
	return len(list)
}

func (list AlbumList) Less(i, j int) bool {
	return list[i].Date.Before(list[j].Date)
}

func (list AlbumList) Swap(i, j int) {
	list[i], list[j] = list[j], list[i]
}

func (store *MediaStore) ListAlbums() (AlbumList, error) {
	files, err := ioutil.ReadDir(store.StoreLocation)
	if err != nil {
		return nil, err
	}

	albums := make([]Album, len(files))
	for i, file := range files {
		album, err := store.GetAlbum(file.Name(), true)
		if err != nil {
			log.Printf("ListAlbum: Cannot extract album info for '%s'", file.Name())
			continue
		}
		albums[i] = *album
	}

	return albums, nil
}

func (store *MediaStore) OpenFile(albumName string, filename string) (*os.File, time.Time, error) {
	if albumName == "" {
		albumName = ".current"
	}

	path := filepath.Join(store.StoreLocation, albumName, filename)
	stat, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}

	fd, err := os.OpenFile(path, os.O_RDONLY, 0600)
	if err != nil {
		return nil, time.Time{}, err
	}

	return fd, stat.ModTime(), nil
}

func (store *MediaStore) GetAlbum(name string, metadataOnly bool) (*Album, error) {
	var album Album
	var filename string
	if name == "" || name == ".current" {
		filename = ".current"
	} else {
		filename = filepath.Base(name)
		album.ID = filename
	}

	if !fileExists(filepath.Join(store.StoreLocation, filename)) {
		return nil, fmt.Errorf("Unknown album '%s'", name)
	}

	err := store.fillAlbumMetadata(filename, &album)
	if err != nil {
		return nil, err
	}

	// If there is a cover media defined, find the corresponding files
	if !album.CoverMedia.IsZero() {
		paths, err := filepath.Glob(filepath.Join(store.StoreLocation, filename, album.CoverMedia.ID+".*"))
		if err == nil { // Best effort
			album.CoverMedia.Files = make([]string, len(paths))
			for j, path := range paths {
				album.CoverMedia.Files[j] = filepath.Base(path)
			}
		}
	}

	if metadataOnly {
		return &album, nil
	}

	err = store.fillAlbumContent(filename, &album)
	if err != nil {
		return nil, err
	}

	if album.CoverMedia.IsZero() {
		album.setDefaultCover()
	}

	return &album, nil
}

func (store *MediaStore) fillAlbumContent(filename string, album *Album) error {
	yamlData, err := ioutil.ReadFile(filepath.Join(store.StoreLocation, filename, "chat.yaml"))
	// if chat.yaml is not there, it may be because there is no media yet
	// It is not an error.
	if err != nil && !os.IsNotExist(err) {
		return nil
	}

	err = yaml.UnmarshalStrict(yamlData, &album.Media)
	if err != nil {
		return err
	}

	// List all files in the album directory, looking for filenames starting by a UUID
	// and build an association between the UUID and the filenames starting with this UUID.
	files, err := ioutil.ReadDir(filepath.Join(store.StoreLocation, filename))
	if err != nil {
		return err
	}
	filesById := make(map[string][]string)
	for _, file := range files {
		n := file.Name()
		if len(n) < 36 {
			// Definitively not a UUID...
			continue
		}

		filesById[n[0:36]] = append(filesById[n[0:36]], n)
	}

	// Find media files matching each id
	for i := range album.Media {
		if files, ok := filesById[album.Media[i].ID]; ok {
			album.Media[i].Files = files
		} else {
			album.Media[i].Files = make([]string, 0)
		}
	}

	return nil
}

func (store *MediaStore) fillAlbumMetadata(filename string, album *Album) error {
	yamlData, err := ioutil.ReadFile(filepath.Join(store.StoreLocation, filename, "meta.yaml"))
	// if meta.yaml is not there, it could be because the album has not yet
	// been initialized. It is not an error.
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	return yaml.UnmarshalStrict(yamlData, album)
}

func (store *MediaStore) GetMedia(albumName string, mediaId string) (*Media, error) {
	album, err := store.GetAlbum(albumName, false)
	if err != nil {
		return nil, err
	}

	for _, media := range album.Media {
		if media.ID == mediaId {
			return &media, nil
		}
	}

	return nil, nil
}

func (store *MediaStore) GetCurrentAlbum() (*Album, error) {
	return store.GetAlbum("", true)
}

func (album *Album) setDefaultCover() {
	if len(album.Media) > 0 {
		var cover Media
		for _, media := range album.Media {
			if media.Type == "photo" { // use the first photo of the album as cover media
				cover = media
				break
			}

			if cover.IsZero() { // otherwise, fallback to the first media
				cover = media
			}
		}
		album.CoverMedia = cover
	}
}

func (store *MediaStore) CloseAlbum() error {
	album, err := store.GetAlbum("", false)
	if err != nil {
		return err
	}

	if album.CoverMedia.ID != "" {
		// Write back the metadata
		yamlData, err := yaml.Marshal(album)
		if err != nil {
			return err
		}

		err = ioutil.WriteFile(filepath.Join(store.StoreLocation, ".current", "meta.yaml"), yamlData, 0644)
		if err != nil {
			return err
		}
	}

	folderName := album.Date.Format("2006-01-02") + "-" + sanitizeAlbumName(album.Title)
	err = os.Rename(filepath.Join(store.StoreLocation, ".current"), filepath.Join(store.StoreLocation, folderName))
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join(store.StoreLocation, ".current"), os.ModePerm)
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
	if fileExists(filepath.Join(store.StoreLocation, ".current")) {
		if fileExists(filepath.Join(store.StoreLocation, ".current", "meta.yaml")) {
			err := store.CloseAlbum()
			if err != nil {
				return err
			}
		}
	}

	err := os.MkdirAll(filepath.Join(store.StoreLocation, ".current"), os.ModePerm)
	if err != nil {
		return err
	}

	metadata := Album{
		Title: title,
		Date:  time.Now(),
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
