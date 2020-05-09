package main

import (
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"
)

type WebInterface struct {
	MediaStore    *MediaStore
	AlbumTemplate *template.Template
	MediaTemplate *template.Template
	IndexTemplate *template.Template
	I18n          I18n
}

type I18n struct {
	SiteName  string
	Bio       string
	LastMedia string
	AllAlbums string
}

func NewWebInterface(statikFS http.FileSystem) (*WebInterface, error) {
	var err error

	web := WebInterface{}
	web.AlbumTemplate, err = getTemplate(statikFS, "/album.html.template", "album")
	if err != nil {
		return nil, err
	}

	web.MediaTemplate, err = getTemplate(statikFS, "/media.html.template", "media")
	if err != nil {
		return nil, err
	}

	web.IndexTemplate, err = getTemplate(statikFS, "/index.html.template", "index")
	if err != nil {
		return nil, err
	}

	return &web, nil
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

func (web *WebInterface) handleFileNotFound(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "File not found", http.StatusNotFound)
}

func (web *WebInterface) handleError(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Internal server error", http.StatusInternalServerError)
}

func (web *WebInterface) handleDisplayAlbum(w http.ResponseWriter, r *http.Request, albumName string) {
	if albumName == "latest" {
		albumName = ""
	}

	album, err := web.MediaStore.GetAlbum(albumName, false)
	if err != nil {
		log.Printf("MediaStore.GetAlbum: %s", err)
		web.handleError(w, r)
		return
	}

	err = web.AlbumTemplate.Execute(w, album)
	if err != nil {
		log.Printf("Template.Execute: %s", err)
		web.handleError(w, r)
		return
	}
}

func (web *WebInterface) handleDisplayIndex(w http.ResponseWriter, r *http.Request) {
	lastAlbum, err := web.MediaStore.GetAlbum("", false)
	if err != nil {
		log.Printf("MediaStore.GetAlbum(latest): %s", err)
		web.handleError(w, r)
		return
	}

	mediaCount := len(lastAlbum.Media)
	if mediaCount >= 5 { // Max 5 media
		mediaCount = 5
	}
	lastMedia := lastAlbum.Media[len(lastAlbum.Media)-mediaCount : len(lastAlbum.Media)]

	albums, err := web.MediaStore.ListAlbums()
	if err != nil {
		log.Printf("MediaStore.ListAlbums: %s", err)
		web.handleError(w, r)
		return
	}

	sort.Sort(sort.Reverse(albums))
	if len(albums) > 0 && albums[0].ID == "" {
		// Latest album should be the first item. Replace it with the one retrieved above
		// with metadata loaded.
		albums[0] = *lastAlbum
	}

	err = web.IndexTemplate.Execute(w, struct {
		I18n      I18n
		LastMedia []Media
		Albums    []Album
	}{
		web.I18n,
		lastMedia,
		albums,
	})
	if err != nil {
		log.Printf("Template.Execute: %s", err)
		web.handleError(w, r)
		return
	}
}

func (web *WebInterface) handleDisplayMedia(w http.ResponseWriter, r *http.Request, albumName string, mediaId string) {
	if albumName == "latest" {
		albumName = ""
	}

	media, err := web.MediaStore.GetMedia(albumName, mediaId)
	if err != nil {
		log.Printf("MediaStore.GetMedia: %s", err)
		web.handleError(w, r)
		return

	}

	if media == nil {
		web.handleFileNotFound(w, r)
		return
	}

	err = web.MediaTemplate.Execute(w, media)
	if err != nil {
		log.Printf("Template.Execute: %s", err)
		web.handleError(w, r)
		return
	}
}

func (web *WebInterface) handleGetMedia(w http.ResponseWriter, r *http.Request, albumName string, mediaFilename string) {
	if albumName == "latest" {
		albumName = ""
	}

	fd, modtime, err := web.MediaStore.OpenFile(albumName, mediaFilename)
	if err != nil {
		log.Printf("MediaStore.OpenFile: %s", err)
		web.handleError(w, r)
		return
	}
	defer fd.Close()
	http.ServeContent(w, r, mediaFilename, modtime, fd)
}

func (web *WebInterface) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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
				web.handleDisplayAlbum(w, r, albumName)
				return
			} else if kind == "raw" && media != "" {
				web.handleGetMedia(w, r, albumName, media)
				return
			} else if kind == "media" && media != "" {
				web.handleDisplayMedia(w, r, albumName, media)
				return
			}
		} else {
			if !strings.HasSuffix(originalPath, "/") {
				http.Redirect(w, r, originalPath+"/", http.StatusMovedPermanently)
				return
			}
			web.handleDisplayIndex(w, r)
			return
		}
	} else if resource == "" {
		http.Redirect(w, r, "/album/", http.StatusMovedPermanently)
		return
	}

	web.handleFileNotFound(w, r)
}
