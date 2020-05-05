package main

import (
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/rakyll/statik/fs"
)

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

func (bot *PhotoBot) ServeWebInterface(listenAddr string, frontend *SecurityFrontend) {
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

	// Put the Web Interface behind the security frontend
	frontend.Protected = bot
	router.Handle("/", frontend)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: router,
	}
	log.Fatal(server.ListenAndServe())
}
