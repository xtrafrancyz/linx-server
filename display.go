package main

import (
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/andreimarcu/linx-server/expiry"
	"github.com/dustin/go-humanize"
	"github.com/flosch/pongo2/v5"
	"github.com/labstack/echo/v4"
	"github.com/microcosm-cc/bluemonday"
	"github.com/russross/blackfriday"
)

const maxDisplayFileSizeBytes = 1024 * 512

func fileDisplayHandler(c echo.Context, fileName string, metadata backends.Metadata) error {
	r := c.Request()

	var expiryHuman string
	if metadata.Expiry != expiry.NeverExpire {
		expiryHuman = humanize.RelTime(time.Now(), metadata.Expiry, "", "")
	}
	sizeHuman := humanize.Bytes(uint64(metadata.Size))
	extra := make(map[string]string)
	var lines []string

	extension := strings.TrimPrefix(filepath.Ext(fileName), ".")

	if strings.EqualFold("application/json", r.Header.Get("Accept")) {
		return c.JSON(http.StatusOK, map[string]string{
			"original_name": metadata.OriginalName,
			"filename":      fileName,
			"direct_url":    getSiteURL(r) + Config.selifPath + fileName,
			"expiry":        strconv.FormatInt(metadata.Expiry.Unix(), 10),
			"size":          strconv.FormatInt(metadata.Size, 10),
			"mimetype":      metadata.Mimetype,
			"sha256sum":     metadata.Sha256sum,
		})
	}

	var tpl string

	if strings.HasPrefix(metadata.Mimetype, "image/") {
		tpl = "display/image.html"

	} else if strings.HasPrefix(metadata.Mimetype, "video/") {
		tpl = "display/video.html"

	} else if strings.HasPrefix(metadata.Mimetype, "audio/") {
		tpl = "display/audio.html"

	} else if metadata.Mimetype == "application/pdf" {
		tpl = "display/pdf.html"

	} else if metadata.Mimetype == "application/vnd.blobkbench.bbmodel+json" {
		tpl = "display/bbmodel.html"

	} else if extension == "story" {
		metadata, reader, err := storageBackend.Get(c.Request().Context(), fileName)
		if err != nil {
			return oopsHandler(c, RespHTML, err.Error())
		}
		defer reader.Close()

		if metadata.Size < maxDisplayFileSizeBytes {
			bytes, err := io.ReadAll(reader)
			if err == nil {
				extra["contents"] = string(bytes)
				lines = strings.Split(extra["contents"], "\n")
				tpl = "display/story.html"
			}
		}

	} else if extension == "md" {
		metadata, reader, err := storageBackend.Get(c.Request().Context(), fileName)
		if err != nil {
			return oopsHandler(c, RespHTML, err.Error())
		}
		defer reader.Close()

		if metadata.Size < maxDisplayFileSizeBytes {
			bytes, err := io.ReadAll(reader)
			if err == nil {
				unsafe := blackfriday.MarkdownCommon(bytes)
				html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)

				extra["contents"] = string(html)
				tpl = "display/md.html"
			}
		}

	} else if strings.HasPrefix(metadata.Mimetype, "text/") || supportedBinExtension(extension) {
		metadata, reader, err := storageBackend.Get(c.Request().Context(), fileName)
		if err != nil {
			return oopsHandler(c, RespHTML, err.Error())
		}
		defer reader.Close()

		if metadata.Size < maxDisplayFileSizeBytes {
			bytes, err := io.ReadAll(reader)
			if err == nil {
				extra["extension"] = extension
				extra["lang_hl"] = extensionToHlLang(extension)
				extra["contents"] = string(bytes)
				tpl = "display/bin.html"
			}
		}
	}

	// Catch other files
	if tpl == "" {
		tpl = "display/file.html"
	}

	if metadata.OriginalName == "" {
		metadata.OriginalName = fileName
	}

	return c.Render(http.StatusOK, tpl, pongo2.Context{
		"mime":           metadata.Mimetype,
		"original_name":  metadata.OriginalName,
		"filename":       fileName,
		"size":           sizeHuman,
		"expiry":         expiryHuman,
		"expirylist":     listExpirationTimes(),
		"extra":          extra,
		"lines":          lines,
		"files":          metadata.ArchiveFiles,
		"siteurl":        strings.TrimSuffix(getSiteURL(r), "/"),
		"keyless_delete": Config.anyoneCanDelete,
	})
}
