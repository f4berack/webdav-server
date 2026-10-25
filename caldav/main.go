package main

import (
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/emersion/go-webdav/caldav"
)

func main() {

	var _ caldav.Backend = (*backend)(nil)

	addr := flag.String("addr", ":8080", "address to listen on")
	dir := flag.String("dir", "./data", "directory where calendars are stored")
	pass := flag.String("pass", "", "password for user alice (empty = no authentication)")
	flag.Parse()

	b := &backend{dir: *dir}
	if err := b.ensureDefault(); err != nil {
		log.Fatal(err)
	}

	var dav http.Handler = &caldav.Handler{Backend: b, Prefix: "/remote.php/dav"}
	dav = b.mkcalendarHandler(dav)

	if *pass != "" {
		dav = basicAuth(username, *pass, dav)
	} else {
		log.Println("WARNING: running without authentication")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/caldav", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, davRoot, http.StatusMovedPermanently)
	})
	mux.Handle("/", dav)

	srv := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("CalDAV server on %s, data in %s, user %s", *addr, *dir, username)
	log.Fatal(srv.ListenAndServe())
}
