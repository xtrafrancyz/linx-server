package main

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"net/url"
	"strings"
)

func AddHeaders(headers []string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			w := c.Response().Writer

			for _, header := range headers {
				headerSplit := strings.SplitN(header, ": ", 2)
				w.Header().Add(headerSplit[0], headerSplit[1])
			}

			return next(c)
		}
	}
}

func getSiteURL(r *http.Request) string {
	if Config.siteURL != "" {
		return Config.siteURL
	} else {
		u := &url.URL{}
		u.Host = r.Host

		if Config.sitePath != "" {
			u.Path = Config.sitePath
		}

		if scheme := r.Header.Get("X-Forwarded-Proto"); scheme != "" {
			u.Scheme = scheme
		} else if Config.certFile != "" || (r.TLS != nil && r.TLS.HandshakeComplete == true) {
			u.Scheme = "https"
		} else {
			u.Scheme = "http"
		}

		return u.String()
	}
}
