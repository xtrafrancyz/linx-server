package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/http/fcgi"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/andreimarcu/linx-server/backends/localfs"
	"github.com/andreimarcu/linx-server/backends/s3"
	"github.com/andreimarcu/linx-server/cleanup"
	"github.com/andreimarcu/linx-server/helpers"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/vharitonsky/iniflags"
)

type headerList []string

func (h *headerList) String() string {
	return strings.Join(*h, ",")
}

func (h *headerList) Set(value string) error {
	*h = append(*h, value)
	return nil
}

var Config struct {
	bind                      string
	filesDir                  string
	metaDir                   string
	siteName                  string
	siteURL                   string
	sitePath                  string
	selifPath                 string
	certFile                  string
	keyFile                   string
	contentSecurityPolicy     string
	fileContentSecurityPolicy string
	referrerPolicy            string
	fileReferrerPolicy        string
	xFrameOptions             string
	maxSize                   int64
	maxExpiry                 uint64
	defaultExpiryCli          uint64
	realIp                    bool
	noLogs                    bool
	allowHotlink              bool
	fastcgi                   bool
	addHeaders                headerList
	noDirectAgents            bool
	s3Endpoint                string
	s3Region                  string
	s3Bucket                  string
	s3ForcePathStyle          bool
	anyoneCanDelete           bool
	accessKeyCookieExpiry     uint64
	customPagesDir            string
	cleanupEveryMinutes       uint64
	forbiddenExtensions       headerList
}

//go:embed static templates
var staticEmbed embed.FS

var storageBackend backends.StorageBackend
var customPages = make(map[string]string)
var customPagesNames = make(map[string]string)

// EchoContentSecurityPolicy creates an Echo middleware for Content Security Policy
func EchoContentSecurityPolicy(policy, referrerPolicy, frame string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			w := c.Response().Writer

			// only add a CSP if one is not already set
			if existing := w.Header().Get(echo.HeaderContentSecurityPolicy); existing == "" {
				w.Header().Add(echo.HeaderContentSecurityPolicy, policy)
			}

			// only add a Referrer Policy if one is not already set
			if existing := w.Header().Get(echo.HeaderReferrerPolicy); existing == "" {
				w.Header().Add(echo.HeaderReferrerPolicy, referrerPolicy)
			}

			w.Header().Set(echo.HeaderXFrameOptions, frame)

			return next(c)
		}
	}
}

func setup() *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.Use(middleware.Recover())

	if Config.realIp {
		e.IPExtractor = echo.ExtractIPFromXFFHeader()
	}

	if !Config.noLogs {
		e.Use(middleware.RequestLoggerWithConfig(middleware.RequestLoggerConfig{
			LogStatus:   true,
			LogURI:      true,
			LogMethod:   true,
			LogRemoteIP: true,
			LogLatency:  true,
			LogValuesFunc: func(c echo.Context, v middleware.RequestLoggerValues) error {
				log.Printf(`%d %s %v from: %s, %v`, v.Status, v.Method, v.URI, v.RemoteIP, v.Latency)
				return nil
			},
		}))
	}

	if Config.contentSecurityPolicy != "" || Config.referrerPolicy != "" || Config.xFrameOptions != "" {
		e.Use(EchoContentSecurityPolicy(
			Config.contentSecurityPolicy,
			Config.referrerPolicy,
			Config.xFrameOptions,
		))
		e.Use(middleware.Secure())
	}
	if len(Config.addHeaders) > 0 {
		e.Use(AddHeaders(Config.addHeaders))
	}

	// make directories if needed
	err := os.MkdirAll(Config.filesDir, 0755)
	if err != nil {
		log.Fatal("Could not create files directory:", err)
	}

	err = os.MkdirAll(Config.metaDir, 0700)
	if err != nil {
		log.Fatal("Could not create metadata directory:", err)
	}

	if Config.siteURL != "" {
		// ensure siteURL ends wth '/'
		if lastChar := Config.siteURL[len(Config.siteURL)-1:]; lastChar != "/" {
			Config.siteURL = Config.siteURL + "/"
		}

		parsedUrl, err := url.Parse(Config.siteURL)
		if err != nil {
			log.Fatal("Could not parse siteurl:", err)
		}

		Config.sitePath = parsedUrl.Path
	} else {
		Config.sitePath = "/"
	}

	Config.selifPath = strings.TrimLeft(Config.selifPath, "/")
	if !strings.HasSuffix(Config.selifPath, "/") {
		Config.selifPath = Config.selifPath + "/"
	}
	if Config.selifPath == "/" {
		Config.selifPath = "selif/"
	}

	if Config.s3Bucket != "" {
		storageBackend = s3.NewS3Backend(Config.s3Bucket, Config.s3Region, Config.s3Endpoint, Config.s3ForcePathStyle)
	} else {
		storageBackend = localfs.NewLocalfsBackend(Config.metaDir, Config.filesDir)
		if Config.cleanupEveryMinutes > 0 {
			go cleanup.PeriodicCleanup(time.Duration(Config.cleanupEveryMinutes)*time.Minute, Config.filesDir, Config.metaDir, Config.noLogs)
		}

	}

	// Template setup
	p2l, err := NewPongo2TemplatesLoader()
	if err != nil {
		log.Fatal("Error: could not load templates", err)
	}
	e.Renderer = p2l

	// Routing setup
	var g *echo.Group
	if Config.sitePath == "/" {
		g = e.Group("")
	} else {
		g = e.Group(Config.sitePath)
	}

	g.GET("/", indexHandler)
	g.GET("/paste", pasteHandler)
	g.GET("/paste/", pasteHandler)
	g.GET("/API", apiDocHandler)
	g.GET("/API/", apiDocHandler)

	g.POST("/upload", uploadPostHandler)
	g.POST("/upload/", uploadPostHandler)
	g.POST("/upload/:name", uploadPostHandler)
	g.PUT("/upload", uploadPutHandler)
	g.PUT("/upload/", uploadPutHandler)
	g.PUT("/upload/:name", uploadPutHandler)

	g.DELETE("/:name", deleteHandler)

	staticfs, _ := fs.Sub(staticEmbed, "static")
	g.StaticFS("/static", staticfs)
	g.FileFS("/favicon.ico", "static/images/favicon.gif", staticEmbed)
	e.FileFS("/robots.txt", "static/robots.txt", staticEmbed)

	// For regex routes, we need to use Echo's regex route syntax
	g.GET("/:name", fileAccessHandler)
	g.POST("/:name", fileAccessHandler)
	g.GET("/"+Config.selifPath+":name", fileServeHandler)

	if Config.customPagesDir != "" {
		initializeCustomPages(Config.customPagesDir)
		for fileName := range customPagesNames {
			g.GET(fileName, makeCustomPageHandler(fileName))
		}
	}

	// Set custom 404 handler
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if he, ok := err.(*echo.HTTPError); ok {
			if he.Code == http.StatusNotFound {
				notFoundHandler(c)
				return
			} else if he.Code == http.StatusUnauthorized {
				unauthorizedHandler(c)
				return
			} else if he.Code == http.StatusBadRequest {
				badRequestHandler(c, RespAUTO, "")
				return
			}
		}
		log.Printf("Error: %v", err)
		e.DefaultHTTPErrorHandler(err, c)
	}

	return e
}

func main() {
	flag.StringVar(&Config.bind, "bind", "127.0.0.1:8080",
		"host to bind to (default: 127.0.0.1:8080)")
	flag.StringVar(&Config.filesDir, "filespath", "files/",
		"path to files directory")
	flag.StringVar(&Config.metaDir, "metapath", "meta/",
		"path to metadata directory")
	flag.BoolVar(&Config.noLogs, "nologs", false,
		"remove stdout output for each request")
	flag.BoolVar(&Config.allowHotlink, "allowhotlink", false,
		"Allow hotlinking of files")
	flag.StringVar(&Config.siteName, "sitename", "",
		"name of the site")
	flag.StringVar(&Config.siteURL, "siteurl", "",
		"site base url (including trailing slash)")
	flag.StringVar(&Config.selifPath, "selifpath", "selif",
		"path relative to site base url where files are accessed directly")
	flag.Int64Var(&Config.maxSize, "maxsize", 4*1024*1024*1024,
		"maximum upload file size in bytes (default 4GB)")
	flag.Uint64Var(&Config.maxExpiry, "maxexpiry", 0,
		"maximum expiration time in seconds (default is 0, which is no expiry)")
	flag.StringVar(&Config.certFile, "certfile", "",
		"path to ssl certificate (for https)")
	flag.StringVar(&Config.keyFile, "keyfile", "",
		"path to ssl key (for https)")
	flag.BoolVar(&Config.realIp, "realip", false,
		"use X-Real-IP/X-Forwarded-For headers as original host")
	flag.BoolVar(&Config.fastcgi, "fastcgi", false,
		"serve through fastcgi")
	flag.StringVar(&Config.contentSecurityPolicy, "contentsecuritypolicy",
		"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'self';",
		"value of default Content-Security-Policy header")
	flag.StringVar(&Config.fileContentSecurityPolicy, "filecontentsecuritypolicy",
		"default-src 'none'; img-src 'self'; object-src 'self'; media-src 'self'; style-src 'self' 'unsafe-inline'; frame-ancestors 'self';",
		"value of Content-Security-Policy header for file access")
	flag.StringVar(&Config.referrerPolicy, "referrerpolicy",
		"same-origin",
		"value of default Referrer-Policy header")
	flag.StringVar(&Config.fileReferrerPolicy, "filereferrerpolicy",
		"same-origin",
		"value of Referrer-Policy header for file access")
	flag.StringVar(&Config.xFrameOptions, "xframeoptions", "SAMEORIGIN",
		"value of X-Frame-Options header")
	flag.Var(&Config.addHeaders, "addheader",
		"Add an arbitrary header to the response. This option can be used multiple times.")
	flag.BoolVar(&Config.noDirectAgents, "nodirectagents", false,
		"disable serving files directly for wget/curl user agents")
	flag.StringVar(&Config.s3Endpoint, "s3-endpoint", "",
		"S3 endpoint")
	flag.StringVar(&Config.s3Region, "s3-region", "",
		"S3 region")
	flag.StringVar(&Config.s3Bucket, "s3-bucket", "",
		"S3 bucket to use for files and metadata")
	flag.BoolVar(&Config.s3ForcePathStyle, "s3-force-path-style", false,
		"Force path-style addressing for S3 (e.g. https://s3.amazonaws.com/linx/example.txt)")
	flag.BoolVar(&Config.anyoneCanDelete, "anyone-can-delete", false,
		"Anyone has delete button on the file page")
	flag.Uint64Var(&Config.accessKeyCookieExpiry, "access-cookie-expiry", 0, "Expiration time for access key cookies in seconds (set 0 to use session cookies)")
	flag.StringVar(&Config.customPagesDir, "custompagespath", "",
		"path to directory containing .md files to render as custom pages")
	flag.Uint64Var(&Config.cleanupEveryMinutes, "cleanup-every-minutes", 0,
		"How often to clean up expired files in minutes (default is 0, which means files will be cleaned up as they are accessed)")
	flag.Var(&Config.forbiddenExtensions, "forbidden-extension",
		"Restrict uploading files with extension (e.g. exe). This option can be used multiple times.")
	flag.Uint64Var(&Config.defaultExpiryCli, "default-expiry-cli", 0, "Default expiry time in seconds for cli uploads (set 0 to use max expiry)")

	iniflags.Parse()

	helpers.RegisterCustomMimeTypes()
	e := setup()

	if Config.fastcgi {
		var listener net.Listener
		var err error
		if Config.bind[0] == '/' {
			listener, err = listenUnixSocket(Config.bind)
		} else {
			listener, err = net.Listen("tcp", Config.bind)
		}
		if err != nil {
			log.Fatal("Could not bind: ", err)
		}

		log.Printf("Serving over fastcgi, bound on %s", Config.bind)
		err = fcgi.Serve(listener, e)
		if err != nil {
			log.Fatal(err)
		}
	} else if Config.certFile != "" {
		log.Printf("Serving over https, bound on %s", Config.bind)
		err := e.StartTLS(Config.bind, Config.certFile, Config.keyFile)
		if err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("Serving over http, bound on %s", Config.bind)
		if strings.HasPrefix(Config.bind, "/") {
			listener, err := listenUnixSocket(Config.bind)
			if err != nil {
				log.Fatal("Could not bind: ", err)
			}
			e.Listener = listener
		}
		err := e.Start(Config.bind)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func listenUnixSocket(path string) (net.Listener, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})

	cleanupSocketFile := func() {
		log.Print("Removing FastCGI socket")
		os.Remove(path)
	}
	defer cleanupSocketFile()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		log.Print("Signal: ", sig)
		cleanupSocketFile()
		os.Exit(0)
	}()

	return listener, err
}
