package main

import (
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	_ "github.com/nmasse-itix/Telegram-Photo-Album-Bot/statik"
	"github.com/rakyll/statik/fs"
	"github.com/spf13/viper"
)

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

// ShiftPath splits off the first component of p, which will be cleaned of
// relative components before processing. head will never contain a slash and
// tail will always be a rooted path without trailing slash.
//
// From https://blog.merovius.de/2017/06/18/how-not-to-use-an-http-router.html
func ShiftPath(p string) (head, tail string) {
	p = path.Clean("/" + p)
	i := strings.Index(p[1:], "/") + 1
	if i <= 0 {
		//log.Printf("head: %s, tail: /", p[1:])
		return p[1:], "/"
	}
	//log.Printf("head: %s, tail: %s", p[1:i], p[i:])
	return p[1:i], p[i:]
}

type WebInterface struct {
	AlbumTemplate *template.Template
	MediaTemplate *template.Template
	IndexTemplate *template.Template
}

func (bot *PhotoBot) ServeWebInterface(listenAddr string) {
	statikFS, err := fs.New()
	if err != nil {
		log.Fatal(err)
	}

	bot.WebInterface.AlbumTemplate, err = getTemplate(statikFS, "/album.html.template", "album")
	if err != nil {
		log.Fatal(err)
	}

	bot.WebInterface.MediaTemplate, err = getTemplate(statikFS, "/media.html.template", "media")
	if err != nil {
		log.Fatal(err)
	}

	bot.WebInterface.IndexTemplate, err = getTemplate(statikFS, "/index.html.template", "index")
	if err != nil {
		log.Fatal(err)
	}

	router := http.NewServeMux()
	router.Handle("/js/", http.FileServer(statikFS))
	router.Handle("/css/", http.FileServer(statikFS))
	router.Handle("/", bot)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: router,
	}
	log.Fatal(server.ListenAndServe())
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
		viper.GetString("SiteName"),
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

	fd, err := bot.MediaStore.OpenFile(albumName, mediaFilename)
	if err != nil {
		log.Printf("MediaStore.OpenFile: %s", err)
		bot.HandleError(w, r)
		return
	}
	defer fd.Close()
	io.Copy(w, fd) // Best effort
}

func (bot *PhotoBot) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	originalPath := r.URL.Path
	var resource string
	resource, r.URL.Path = ShiftPath(r.URL.Path)

	switch r.Method {
	case "GET":
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

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	bot.HandleFileNotFound(w, r)
}
