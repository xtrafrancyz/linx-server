package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/andreimarcu/linx-server/expiry"
	"github.com/dchest/uniuri"
	"github.com/gabriel-vasile/mimetype"
	"github.com/microcosm-cc/bluemonday"
	"github.com/zenazn/goji/web"
)

var FileTooLargeError = errors.New("File too large.")
var fileBlacklist = map[string]bool{
	"favicon.ico":     true,
	"index.htm":       true,
	"index.html":      true,
	"index.php":       true,
	"robots.txt":      true,
	"crossdomain.xml": true,
}

// Describes metadata directly from the user request
type UploadRequest struct {
	src       io.Reader
	size      int64
	filename  string
	expiry    time.Duration // Seconds until expiry, 0 = never
	deleteKey string        // Empty string if not defined
	accessKey string        // Empty string if not defined
}

// Metadata associated with a file as it would actually be stored
type Upload struct {
	Filename string // Final filename on disk
	Metadata backends.Metadata
}

func uploadPostHandler(c web.C, w http.ResponseWriter, r *http.Request) {
	if !strictReferrerCheck(r, getSiteURL(r), []string{"Linx-Delete-Key", "Linx-Expiry", "X-Requested-With"}) {
		badRequestHandler(c, w, r, RespAUTO, "")
		return
	}

	upReq := UploadRequest{}
	uploadHeaderProcess(r, &upReq)

	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		file, headers, err := r.FormFile("file")
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll()
		}
		if err != nil {
			oopsHandler(c, w, r, RespHTML, "Could not upload file.")
			return
		}
		defer file.Close()

		upReq.src = file
		upReq.size = headers.Size
		upReq.filename = headers.Filename
	} else {
		if r.PostFormValue("content") == "" {
			badRequestHandler(c, w, r, RespAUTO, "Empty file")
			return
		}
		extension := r.PostFormValue("extension")
		if extension == "" {
			extension = "txt"
		}
		content := r.PostFormValue("content")
		upReq.src = strings.NewReader(content)
		upReq.size = int64(len(content))
		upReq.filename = r.PostFormValue("filename") + "." + extension
	}

	cli := cliUserAgentRe.MatchString(r.Header.Get("User-Agent"))
	upReq.expiry = parseExpiry(r.PostFormValue("expires"), cli)
	upReq.accessKey = r.PostFormValue(accessKeyParamName)

	upload, err := processUpload(upReq)

	if strings.EqualFold("application/json", r.Header.Get("Accept")) {
		if err == FileTooLargeError || err == backends.FileEmptyError {
			badRequestHandler(c, w, r, RespJSON, err.Error())
			return
		} else if err != nil {
			oopsHandler(c, w, r, RespJSON, "Could not upload file: "+err.Error())
			return
		}

		js := generateJSONresponse(upload, r)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Write(js)
	} else {
		if err == FileTooLargeError || err == backends.FileEmptyError {
			badRequestHandler(c, w, r, RespHTML, err.Error())
			return
		} else if err != nil {
			oopsHandler(c, w, r, RespHTML, "Could not upload file: "+err.Error())
			return
		}

		http.Redirect(w, r, Config.sitePath+upload.Filename, 303)
	}
}

func uploadPutHandler(c web.C, w http.ResponseWriter, r *http.Request) {
	upReq := UploadRequest{}
	uploadHeaderProcess(r, &upReq)

	defer r.Body.Close()
	upReq.filename = c.URLParams["name"]
	upReq.src = http.MaxBytesReader(w, r.Body, Config.maxSize)

	upload, err := processUpload(upReq)

	if strings.EqualFold("application/json", r.Header.Get("Accept")) {
		if err == FileTooLargeError || err == backends.FileEmptyError {
			badRequestHandler(c, w, r, RespJSON, err.Error())
			return
		} else if err != nil {
			oopsHandler(c, w, r, RespJSON, "Could not upload file: "+err.Error())
			return
		}

		js := generateJSONresponse(upload, r)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Write(js)
	} else {
		if err == FileTooLargeError || err == backends.FileEmptyError {
			badRequestHandler(c, w, r, RespPLAIN, err.Error())
			return
		} else if err != nil {
			oopsHandler(c, w, r, RespPLAIN, "Could not upload file: "+err.Error())
			return
		}

		fmt.Fprintf(w, "%s\n", getSiteURL(r)+upload.Filename)
	}
}

func uploadRemote(c web.C, w http.ResponseWriter, r *http.Request) {
	if Config.remoteAuthFile != "" {
		key := r.FormValue("key")
		if key == "" && Config.basicAuth {
			_, password, ok := r.BasicAuth()
			if ok {
				key = password
			}
		}
		result, err := checkAuth(remoteAuthKeys, key)
		if err != nil || !result {
			if Config.basicAuth {
				badAuthorizationHandler(w, r)
			} else {
				unauthorizedHandler(c, w, r)
			}
			return
		}
	}

	if r.FormValue("url") == "" {
		http.Redirect(w, r, Config.sitePath, 303)
		return
	}

	upReq := UploadRequest{}
	grabUrl, _ := url.Parse(r.FormValue("url"))
	directURL := r.FormValue("direct_url") == "yes"

	resp, err := http.Get(grabUrl.String())
	if err != nil {
		oopsHandler(c, w, r, RespAUTO, "Could not retrieve URL")
		return
	}

	upReq.filename = filepath.Base(grabUrl.Path)
	upReq.src = http.MaxBytesReader(w, resp.Body, Config.maxSize)
	upReq.deleteKey = r.FormValue("deletekey")
	upReq.accessKey = r.FormValue(accessKeyParamName)
	upReq.expiry = parseExpiry(r.FormValue("expiry"), true)

	upload, err := processUpload(upReq)

	if strings.EqualFold("application/json", r.Header.Get("Accept")) {
		if err != nil {
			oopsHandler(c, w, r, RespJSON, "Could not upload file: "+err.Error())
			return
		}

		js := generateJSONresponse(upload, r)
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		w.Write(js)
	} else {
		if err != nil {
			oopsHandler(c, w, r, RespHTML, "Could not upload file: "+err.Error())
			return
		}

		if directURL {
			http.Redirect(w, r, Config.sitePath+Config.selifPath+upload.Filename, 303)
		} else {
			http.Redirect(w, r, Config.sitePath+upload.Filename, 303)
		}
	}
}

func uploadHeaderProcess(r *http.Request, upReq *UploadRequest) {
	upReq.deleteKey = r.Header.Get("Linx-Delete-Key")
	upReq.accessKey = r.Header.Get(accessKeyHeaderName)

	// Get seconds until expiry. Non-integer responses never expire.
	expStr := r.Header.Get("Linx-Expiry")
	cli := cliUserAgentRe.MatchString(r.Header.Get("User-Agent"))
	upReq.expiry = parseExpiry(expStr, cli)
}

func processUpload(upReq UploadRequest) (upload Upload, err error) {
	if upReq.size > Config.maxSize {
		return upload, FileTooLargeError
	}
	if len(upReq.filename) > 255 {
		return upload, errors.New("filename too large")
	}
	upReq.filename = bluemonday.StrictPolicy().Sanitize(upReq.filename)

	// Determine the appropriate filename
	barename, extension := barePlusExt(upReq.filename)

	var header []byte
	if len(extension) == 0 {
		// Pull the first 512 bytes off for use in MIME detection
		header = make([]byte, 512)
		n, _ := upReq.src.Read(header)
		if n == 0 {
			return upload, backends.FileEmptyError
		}
		header = header[:n]

		// Determine the type of file from header
		kind := mimetype.Detect(header)
		if len(kind.Extension()) < 2 {
			extension = "file"
		} else {
			extension = kind.Extension()[1:] // remove leading "."
		}
	}

	for _, e := range Config.forbiddenExtensions {
		if extension == e {
			return upload, errors.New("forbidden file extension")
		}
	}

	for {
		slug := generateBarename()
		upload.Filename = strings.Join([]string{slug, extension}, ".")
		exists, err := storageBackend.Exists(upload.Filename)
		if err != nil {
			return upload, err
		}
		if !exists {
			break
		}
	}

	if fileBlacklist[strings.ToLower(upload.Filename)] {
		return upload, errors.New("Prohibited filename")
	}

	// Get the rest of the metadata needed for storage
	var fileExpiry time.Time
	if upReq.expiry == 0 {
		fileExpiry = expiry.NeverExpire
	} else {
		fileExpiry = time.Now().Add(upReq.expiry)
	}

	if upReq.deleteKey == "" {
		upReq.deleteKey = uniuri.NewLen(30)
	}

	if len(barename) == 0 {
		upReq.filename = upload.Filename
	}

	upload.Metadata, err = storageBackend.Put(upload.Filename, upReq.filename, io.MultiReader(bytes.NewReader(header), upReq.src), fileExpiry, upReq.deleteKey, upReq.accessKey)
	if err != nil {
		return upload, err
	}

	return
}

func generateBarename() string {
	return uniuri.NewLenChars(10, []byte("abcdefghijklmnopqrstuvwxyz0123456789"))
}

func generateJSONresponse(upload Upload, r *http.Request) []byte {
	js, _ := json.Marshal(map[string]string{
		"url":           getSiteURL(r) + upload.Filename,
		"direct_url":    getSiteURL(r) + Config.selifPath + upload.Filename,
		"filename":      upload.Filename,
		"original_name": upload.Metadata.OriginalName,
		"delete_key":    upload.Metadata.DeleteKey,
		"access_key":    upload.Metadata.AccessKey,
		"expiry":        strconv.FormatInt(upload.Metadata.Expiry.Unix(), 10),
		"size":          strconv.FormatInt(upload.Metadata.Size, 10),
		"mimetype":      upload.Metadata.Mimetype,
		"sha256sum":     upload.Metadata.Sha256sum,
	})

	return js
}

var bareRe = regexp.MustCompile(`[^A-Za-z0-9\-]`)
var extRe = regexp.MustCompile(`[^A-Za-z0-9\-\.]`)
var compressedExts = map[string]bool{
	".bz2": true,
	".gz":  true,
	".xz":  true,
}
var archiveExts = map[string]bool{
	".tar": true,
}

func barePlusExt(filename string) (barename, extension string) {
	filename = strings.TrimSpace(filename)
	filename = strings.ToLower(filename)

	extension = path.Ext(filename)
	barename = filename[:len(filename)-len(extension)]
	if compressedExts[extension] {
		ext2 := path.Ext(barename)
		if archiveExts[ext2] {
			barename = barename[:len(barename)-len(ext2)]
			extension = ext2 + extension
		}
	}

	extension = extRe.ReplaceAllString(extension, "")
	barename = bareRe.ReplaceAllString(barename, "")

	extension = strings.Trim(extension, "-.")
	barename = strings.Trim(barename, "-")

	return
}

func parseExpiry(expStr string, cli bool) time.Duration {
	fallback := Config.maxExpiry
	if cli && Config.defaultExpiryCli > 0 {
		fallback = Config.defaultExpiryCli
	}
	if expStr == "" {
		return time.Duration(fallback) * time.Second
	}
	fileExpiry, err := strconv.ParseUint(expStr, 10, 64)
	if err != nil {
		return time.Duration(fallback) * time.Second
	}
	if fileExpiry == 0 {
		fileExpiry = fallback
	}
	if Config.maxExpiry > 0 && fileExpiry > Config.maxExpiry {
		fileExpiry = Config.maxExpiry
	}
	return time.Duration(fileExpiry) * time.Second
}
