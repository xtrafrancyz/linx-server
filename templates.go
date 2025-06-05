package main

import (
	"bytes"
	"errors"
	"github.com/labstack/echo/v4"
	"io"
	"path"
	"path/filepath"
	"strings"

	"github.com/flosch/pongo2/v5"
)

type Pongo2Loader struct {
	parsed map[string]*pongo2.Template
}

func NewPongo2TemplatesLoader() (*Pongo2Loader, error) {
	loader := &Pongo2Loader{}
	err := loader.compile()
	return loader, err
}

func (fs *Pongo2Loader) Get(path string) (io.Reader, error) {
	myBytes, err := staticEmbed.ReadFile("templates/" + path)
	if err != nil {
		return nil, err
	}

	return bytes.NewReader(myBytes), nil
}

func (fs *Pongo2Loader) Abs(base, name string) string {
	return path.Join(filepath.Dir(base), name)
}

func (fs *Pongo2Loader) compile() error {
	templates := []string{
		"index.html",
		"paste.html",
		"API.html",
		"400.html",
		"401.html",
		"404.html",
		"oops.html",
		"access.html",
		"custom_page.html",

		"display/audio.html",
		"display/image.html",
		"display/video.html",
		"display/pdf.html",
		"display/bin.html",
		"display/story.html",
		"display/md.html",
		"display/file.html",
		"display/bbmodel.html",
	}

	fs.parsed = make(map[string]*pongo2.Template)

	tSet := pongo2.NewSet("templates", fs)

	for _, tName := range templates {
		tpl, err := tSet.FromFile(tName)
		if err != nil {
			return err
		}

		fs.parsed[tName] = tpl
	}

	return nil
}

func (fs *Pongo2Loader) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	tpl, ok := fs.parsed[name]
	if !ok {
		return errors.New("Template not found: " + name)
	}

	var context pongo2.Context
	if context, ok = data.(pongo2.Context); !ok {
		context = pongo2.Context{}
	}

	if Config.siteName == "" {
		parts := strings.Split(c.Request().Host, ":")
		context["sitename"] = parts[0]
	} else {
		context["sitename"] = Config.siteName
	}

	context["sitepath"] = Config.sitePath
	context["selifpath"] = Config.selifPath
	context["custom_pages_names"] = customPagesNames

	return tpl.ExecuteWriter(context, w)
}
