package main

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/andreimarcu/linx-server/expiry"
	"github.com/andreimarcu/linx-server/httputil"
	"github.com/labstack/echo/v4"
)

func fileServeHandler(c echo.Context) error {
	fileName := c.Param("name")

	metadata, err := checkFile(fileName)
	if err == backends.NotFoundErr {
		return notFoundHandler(c)
	} else if err != nil {
		return oopsHandler(c, RespAUTO, "Corrupt metadata.")
	}

	r := c.Request()
	w := c.Response().Writer

	if src, err := checkAccessKey(r, &metadata); err != nil {
		// remove invalid cookie
		if src == accessKeySourceCookie {
			setAccessKeyCookies(w, getSiteURL(r), fileName, "", time.Unix(0, 0))
		}
		return echo.ErrUnauthorized
	}

	if !Config.allowHotlink {
		referer := r.Header.Get("Referer")
		u, _ := url.Parse(referer)
		p, _ := url.Parse(getSiteURL(r))
		if referer != "" && !sameOrigin(u, p) {
			return c.Redirect(303, Config.sitePath+fileName)
		}
	}

	if Config.fileContentSecurityPolicy != "" {
		c.Response().Header().Set("Content-Security-Policy", Config.fileContentSecurityPolicy)
	}
	if Config.fileReferrerPolicy != "" {
		c.Response().Header().Set("Referrer-Policy", Config.fileReferrerPolicy)
	}

	if metadata.OriginalName != "" {
		c.Response().Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", strings.Replace(metadata.OriginalName, `"`, ``, -1)))
	}
	c.Response().Header().Set("Content-Type", metadata.Mimetype)
	c.Response().Header().Set("Content-Length", strconv.FormatInt(metadata.Size, 10))
	c.Response().Header().Set("Etag", fmt.Sprintf("\"%s\"", metadata.Sha256sum))
	c.Response().Header().Set("Cache-Control", "public, no-cache")

	modtime := time.Unix(0, 0)
	if done := httputil.CheckPreconditions(w, r, modtime); done == true {
		return nil
	}

	if r.Method != "HEAD" {
		err = storageBackend.ServeFile(fileName, w, r)
		if err != nil {
			return oopsHandler(c, RespAUTO, err.Error())
		}
	}

	return nil
}

func checkFile(filename string) (metadata backends.Metadata, err error) {
	metadata, err = storageBackend.Head(filename)
	if err != nil {
		return
	}

	if expiry.IsTsExpired(metadata.Expiry) {
		storageBackend.Delete(filename)
		err = backends.NotFoundErr
		return
	}

	return
}
