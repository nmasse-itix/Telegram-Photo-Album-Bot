package main

import (
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	_ "github.com/nmasse-itix/Telegram-Photo-Album-Bot/statik"
)

type WebInterface struct {
	AlbumTemplate *template.Template
	MediaTemplate *template.Template
	IndexTemplate *template.Template
	SiteName      string
}

func slurpFile(statikFS http.FileSystem, filename string) (string, error) {
	fd, err := statikFS.Open(filename)
	if err != nil {
		return "", err
	}
	defer fd.Close()

	content, err := ioutil.ReadAll(fd)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

func getTemplate(statikFS http.FileSystem, filename string, name string) (*template.Template, error) {
	tmpl := template.New(name)
	content, err := slurpFile(statikFS, filename)
	if err != nil {
		return nil, err
	}

	customFunctions := template.FuncMap{
		"video": func(files []string) string {
			for _, file := range files {
				if strings.HasSuffix(file, ".mp4") {
					return file
				}
			}
			return ""
		},
		"photo": func(files []string) string {
			for _, file := range files {
				if strings.HasSuffix(file, ".jpeg") {
					return file
				}
			}
			return ""
		},
		"short": func(t time.Time) string {
			return t.Format("2006-01")
		},
	}

	return tmpl.Funcs(customFunctions).Parse(content)
}

func (bot *PhotoBot) HandleFileNotFound(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "File not found", http.StatusNotFound)
}

func (bot *PhotoBot) HandleError(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

func (bot *PhotoBot) HandleDisplayAlbum(w http.ResponseWriter, r *http.Request, albumName string) {
	if albumName == "latest" {
		albumName = ""
	}

	album, err := bot.MediaStore.GetAlbum(albumName, false)
	if err != nil {
		log.Printf("MediaStore.GetAlbum: %s", err)
		bot.HandleError(w, r)
		return
	}

	err = bot.WebInterface.AlbumTemplate.Execute(w, album)
	if err != nil {
		log.Printf("Template.Execute: %s", err)
		bot.HandleError(w, r)
		return
	}
}

func (bot *PhotoBot) HandleDisplayIndex(w http.ResponseWriter, r *http.Request) {
	albums, err := bot.MediaStore.ListAlbums()
	if err != nil {
		log.Printf("MediaStore.ListAlbums: %s", err)
		bot.HandleError(w, r)
		return
	}

	sort.Sort(sort.Reverse(albums))
	err = bot.WebInterface.IndexTemplate.Execute(w, struct {
		Title  string
		Albums []Album
	}{
		bot.WebInterface.SiteName,
		albums,
	})
	if err != nil {
		log.Printf("Template.Execute: %s", err)
		bot.HandleError(w, r)
		return
	}
}

func (bot *PhotoBot) HandleDisplayMedia(w http.ResponseWriter, r *http.Request, albumName string, mediaId string) {
	if albumName == "latest" {
		albumName = ""
	}

	media, err := bot.MediaStore.GetMedia(albumName, mediaId)
	if err != nil {
		log.Printf("MediaStore.GetMedia: %s", err)
		bot.HandleError(w, r)
		return

	}

	if media == nil {
		bot.HandleFileNotFound(w, r)
		return
	}

	err = bot.WebInterface.MediaTemplate.Execute(w, media)
	if err != nil {
		log.Printf("Template.Execute: %s", err)
		bot.HandleError(w, r)
		return
	}
}

func (bot *PhotoBot) HandleGetMedia(w http.ResponseWriter, r *http.Request, albumName string, mediaFilename string) {
	if albumName == "latest" {
		albumName = ""
	}

	fd, modtime, err := bot.MediaStore.OpenFile(albumName, mediaFilename)
	if err != nil {
		log.Printf("MediaStore.OpenFile: %s", err)
		bot.HandleError(w, r)
		return
	}
	defer fd.Close()
	http.ServeContent(w, r, mediaFilename, modtime, fd)
}

func (bot *PhotoBot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	originalPath := r.URL.Path
	var resource string
	resource, r.URL.Path = ShiftPath(r.URL.Path)

	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if resource == "album" {
		var albumName, kind, media string
		albumName, r.URL.Path = ShiftPath(r.URL.Path)
		kind, r.URL.Path = ShiftPath(r.URL.Path)
		media, r.URL.Path = ShiftPath(r.URL.Path)
		if albumName != "" {
			if kind == "" && media == "" {
				if !strings.HasSuffix(originalPath, "/") {
					http.Redirect(w, r, originalPath+"/", http.StatusMovedPermanently)
					return
				}
				bot.HandleDisplayAlbum(w, r, albumName)
				return
			} else if kind == "raw" && media != "" {
				bot.HandleGetMedia(w, r, albumName, media)
				return
			} else if kind == "media" && media != "" {
				bot.HandleDisplayMedia(w, r, albumName, media)
				return
			}
		} else {
			if !strings.HasSuffix(originalPath, "/") {
				http.Redirect(w, r, originalPath+"/", http.StatusMovedPermanently)
				return
			}
			bot.HandleDisplayIndex(w, r)
			return
		}
	} else if resource == "" {
		http.Redirect(w, r, "/album/", http.StatusMovedPermanently)
		return
	}

	bot.HandleFileNotFound(w, r)
}
