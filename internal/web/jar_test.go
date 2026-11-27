package web

import (
	"net/http"
	"net/http/cookiejar"
)

func newJar() http.CookieJar {
	j, _ := cookiejar.New(nil)
	return j
}
