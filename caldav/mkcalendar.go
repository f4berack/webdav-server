package main

import (
	"encoding/xml"
	"net/http"
)

func (b *backend) mkcalendarHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "MKCALENDAR" {
			next.ServeHTTP(w, r)
			return
		}
		var body struct {
			Name string `xml:"set>prop>displayname"`
			Desc string `xml:"set>prop>calendar-description"`
		}
		_ = xml.NewDecoder(r.Body).Decode(&body) // an empty body is fine

		code, err := b.mkcal(r.URL.Path, body.Name, body.Desc)
		if err != nil {
			http.Error(w, err.Error(), code)
			return
		}
		w.WriteHeader(code)
	})
}
