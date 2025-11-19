package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

	metadata, err := checkFile(c.Request().Context(), fileName)
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

	if c.QueryParam("blockbench_redirect") == "1" {
		return redirectBlockbenchHandler(c, fileName, metadata)
	}

	return fileDisplayHandler(c, fileName, metadata)
}

func redirectBlockbenchHandler(c echo.Context, fileName string, metadata backends.Metadata) error {
	if metadata.Mimetype != "application/vnd.blobkbench.bbmodel+json" {
		return oopsHandler(c, RespHTML, "Invalid .bbmodel file.")
	}
	if metadata.Size > 10*1024*1024 { // 10 MB limit
		return oopsHandler(c, RespHTML, "File too large for Blockbench upload.")
	}

	metadata, reader, err := storageBackend.Get(c.Request().Context(), fileName)
	if err != nil {
		return oopsHandler(c, RespHTML, err.Error())
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return oopsHandler(c, RespHTML, "Failed to read file: "+err.Error())
	}

	uploadBody, _ := json.Marshal(map[string]string{
		"expire_time": "10m",
		"model":       string(data),
		"name":        metadata.OriginalName,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://blckbn.ch/api/model", bytes.NewReader(uploadBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oopsHandler(c, RespHTML, "Failed to upload to Blockbench: "+err.Error())
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return oopsHandler(c, RespHTML, "Blockbench upload failed with status: "+resp.Status)
	}
	var result map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return oopsHandler(c, RespHTML, "Failed to parse Blockbench response: "+err.Error())
	}
	if result["text"] == "Model uploaded successfully" {
		redirectURL := "https://blckbn.ch/" + result["id"]
		return c.Redirect(http.StatusSeeOther, redirectURL)
	} else {
		return oopsHandler(c, RespHTML, "Blockbench upload failed: "+result["text"])
	}
}
