package main

import (
	"net/http"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/labstack/echo/v4"
)

func deleteHandler(c echo.Context) error {
	requestKey := c.Request().Header.Get("Linx-Delete-Key")

	filename := c.Param("name")

	// Ensure that file exists and delete key is correct
	metadata, err := storageBackend.Head(filename)
	if err == backends.NotFoundErr {
		return echo.ErrNotFound // 404 - file doesn't exist
	} else if err != nil {
		return echo.ErrUnauthorized // 401 - no metadata available
	}

	if Config.anyoneCanDelete || metadata.DeleteKey == requestKey {
		err = storageBackend.Delete(filename)
		if err != nil {
			return oopsHandler(c, RespPLAIN, "Could not delete")
		}

		return c.String(http.StatusOK, "DELETED")

	} else {
		return echo.ErrUnauthorized // 401 - wrong delete key
	}
}
