package main

import (
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v5"
)

type RespType int

const (
	RespPLAIN RespType = iota
	RespJSON
	RespHTML
	RespAUTO
)

func indexHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "index.html", pongo2.Context{
		"maxsize":    Config.maxSize,
		"expirylist": listExpirationTimes(),
	})
}

func pasteHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "paste.html", pongo2.Context{
		"expirylist": listExpirationTimes(),
	})
}

func apiDocHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "API.html", pongo2.Context{
		"siteurl":        getSiteURL(c.Request()),
		"keyless_delete": Config.anyoneCanDelete,
	})
}

func makeCustomPageHandler(fileName string) echo.HandlerFunc {
	return func(c echo.Context) error {
		return c.Render(http.StatusOK, "custom_page.html", pongo2.Context{
			"siteurl":  getSiteURL(c.Request()),
			"contents": customPages[fileName],
			"filename": fileName,
			"pagename": customPagesNames[fileName],
		})
	}
}

func notFoundHandler(c echo.Context) error {
	return c.Render(http.StatusNotFound, "404.html", nil)
}

func oopsHandler(c echo.Context, rt RespType, msg string) error {
	if msg == "" {
		msg = "Oops! Something went wrong..."
	}

	if rt == RespHTML {
		return c.Render(500, "oops.html", pongo2.Context{"msg": msg})
	} else if rt == RespPLAIN {
		return c.String(500, msg)
	} else if rt == RespJSON {
		return c.JSON(500, map[string]string{
			"error": msg,
		})
	} else if rt == RespAUTO {
		if strings.EqualFold("application/json", c.Request().Header.Get("Accept")) {
			return oopsHandler(c, RespJSON, msg)
		} else {
			return oopsHandler(c, RespHTML, msg)
		}
	}
	return nil
}

func badRequestHandler(c echo.Context, rt RespType, msg string) error {
	if rt == RespHTML {
		return c.Render(http.StatusBadRequest, "400.html", pongo2.Context{"msg": msg})
	} else if rt == RespPLAIN {
		return c.String(http.StatusBadRequest, msg)
	} else if rt == RespJSON {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": msg,
		})
	} else if rt == RespAUTO {
		if strings.EqualFold("application/json", c.Request().Header.Get("Accept")) {
			return badRequestHandler(c, RespJSON, msg)
		} else {
			return badRequestHandler(c, RespHTML, msg)
		}
	}
	return nil
}

func unauthorizedHandler(c echo.Context) error {
	return c.Render(http.StatusUnauthorized, "401.html", nil)
}
