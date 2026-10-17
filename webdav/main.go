package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"golang.org/x/net/webdav"
)

func main() {

	addr := flag.String("addr", ":8080", "address to listen on")
	dir := flag.String("dir", "./data", "directory to serve")
	user := flag.String("user", "", "username (empty = no authentication)")
	pass := flag.String("pass", "", "password")

	flag.Parse()

	var handler http.Handler = &webdav.Handler{
		FileSystem: webdav.Dir("./data"),
		LockSystem: webdav.NewMemLS(),
		Logger: func(r *http.Request, err error) {
			if err != nil {
				log.Printf("%s %s -> ERROR: %v", r.Method, r.URL.Path, err)
				return
			}
			log.Printf("%s %s", r.Method, r.URL.Path)
		},
	}

	if *user != "" {
		handler = basicAuth(*user, *pass, handler)
	} else {
		log.Println("WARNING: running without authentication")
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	http.Handle("/", handler)

	log.Printf("WebDAV server on %s, serving %s", *addr, *dir)
	log.Fatal(srv.ListenAndServe())

}
