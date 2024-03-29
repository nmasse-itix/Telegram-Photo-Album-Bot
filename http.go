package main

import (
	"net/http"
	"path"
	"strings"
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

func ServeWebInterface(listenAddr string, webInterface http.Handler, staticFiles http.FileSystem) error {
	router := http.NewServeMux()
	router.Handle("/js/", http.FileServer(staticFiles))
	router.Handle("/css/", http.FileServer(staticFiles))
	router.Handle("/", webInterface)

	server := &http.Server{
		Addr:    listenAddr,
		Handler: router,
	}
	return server.ListenAndServe()
}
