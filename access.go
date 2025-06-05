package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/flosch/pongo2/v5"
	"github.com/labstack/echo/v4"
)

type accessKeySource int

const (
	accessKeySourceNone accessKeySource = iota
	accessKeySourceCookie
	accessKeySourceHeader
	accessKeySourceForm
	accessKeySourceQuery
)

const accessKeyHeaderName = "Linx-Access-Key"
const accessKeyParamName = "access_key"

var (
	errInvalidAccessKey = errors.New("invalid access key")

	cliUserAgentRe = regexp.MustCompile("(?i)(lib)?curl|wget|java|python|go-http-client")
)

func checkAccessKey(r *http.Request, metadata *backends.Metadata) (accessKeySource, error) {
	key := metadata.AccessKey
	if key == "" {
		return accessKeySourceNone, nil
	}

	cookieKey, err := r.Cookie(accessKeyHeaderName)
	if err == nil {
		if cookieKey.Value == key {
			return accessKeySourceCookie, nil
		}
		return accessKeySourceCookie, errInvalidAccessKey
	}

	headerKey := r.Header.Get(accessKeyHeaderName)
	if headerKey == key {
		return accessKeySourceHeader, nil
	} else if headerKey != "" {
		return accessKeySourceHeader, errInvalidAccessKey
	}

	formKey := r.PostFormValue(accessKeyParamName)
	if formKey == key {
		return accessKeySourceForm, nil
	} else if formKey != "" {
		return accessKeySourceForm, errInvalidAccessKey
	}

	queryKey := r.URL.Query().Get(accessKeyParamName)
	if queryKey == key {
		return accessKeySourceQuery, nil
	} else if formKey != "" {
		return accessKeySourceQuery, errInvalidAccessKey
	}

	return accessKeySourceNone, errInvalidAccessKey
}

func setAccessKeyCookies(w http.ResponseWriter, siteURL, fileName, value string, expires time.Time) {
	u, err := url.Parse(siteURL)
	if err != nil {
		log.Printf("cant parse siteURL (%v): %v", siteURL, err)
		return
	}

	cookie := http.Cookie{
		Name:     accessKeyHeaderName,
		Value:    value,
		HttpOnly: true,
		Domain:   u.Hostname(),
		Expires:  expires,
	}

	cookie.Path = path.Join(u.Path, fileName)
	http.SetCookie(w, &cookie)

	cookie.Path = path.Join(u.Path, Config.selifPath, fileName)
	http.SetCookie(w, &cookie)
}

func fileAccessHandler(c echo.Context) error {
	r := c.Request()
	w := c.Response().Writer

	if !Config.noDirectAgents && cliUserAgentRe.MatchString(r.Header.Get("User-Agent")) && !strings.EqualFold("application/json", r.Header.Get("Accept")) {
		return fileServeHandler(c)
	}

	fileName := c.Param("name")

	metadata, err := checkFile(fileName)
	if err == backends.NotFoundErr {
		return notFoundHandler(c)
	} else if err != nil {
		return oopsHandler(c, RespAUTO, "Corrupt metadata.")
	}

	if src, err := checkAccessKey(r, &metadata); err != nil {
		// remove invalid cookie
		if src == accessKeySourceCookie {
			setAccessKeyCookies(w, getSiteURL(r), fileName, "", time.Unix(0, 0))
		}

		if strings.EqualFold("application/json", r.Header.Get("Accept")) {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": errInvalidAccessKey.Error(),
			})
		}

		return c.Render(http.StatusOK, "access.html", pongo2.Context{
			"filename":   fileName,
			"accesspath": fileName,
		})
	}

	if metadata.AccessKey != "" {
		var expiry time.Time
		if Config.accessKeyCookieExpiry != 0 {
			expiry = time.Now().Add(time.Duration(Config.accessKeyCookieExpiry) * time.Second)
		}
		setAccessKeyCookies(w, getSiteURL(r), fileName, metadata.AccessKey, expiry)
	}

	return fileDisplayHandler(c, fileName, metadata)
}
