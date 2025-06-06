package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"
)

type RespOkJSON struct {
	Filename      string
	Url           string
	Delete_Key    string
	Expiry        string
	Size          string
	Original_Name string
}

type RespErrJSON struct {
	Error string
}

func TestSetup(t *testing.T) {
	Config.siteURL = "http://linx.example.org/"
	Config.filesDir = path.Join(os.TempDir(), generateBarename())
	Config.metaDir = Config.filesDir + "_meta"
	Config.maxSize = 1024 * 1024 * 1024
	Config.noLogs = true
	Config.siteName = "linx"
}

func TestIndex(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "Click or Drop file") {
		t.Fatal("String 'Click or Drop file' not found in index response")
	}
}

func TestIndexStandardMaxExpiry(t *testing.T) {
	mux := setup()
	Config.maxExpiry = 60
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if strings.Contains(w.Body.String(), ">1 hour</object>") {
		t.Fatal("String '>1 hour</object>' found in index response")
	}

	Config.maxExpiry = 0
}

func TestIndexWeirdMaxExpiry(t *testing.T) {
	mux := setup()
	Config.maxExpiry = 1500
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if strings.Contains(w.Body.String(), ">never</object>") {
		t.Fatal("String '>never</object>' found in index response")
	}

	Config.maxExpiry = 0
}

func TestAddHeader(t *testing.T) {
	Config.addHeaders = []string{"Linx-Test: It works!"}

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/", nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Header().Get("Linx-Test") != "It works!" {
		t.Fatal("Header 'Linx-Test: It works!' not found in index response")
	}
}

func TestNotFound(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/url/should/not/exist", nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("Expected 404, got %d", w.Code)
	}
}

func TestFileNotFound(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()

	req, err := http.NewRequest("GET", "/"+Config.selifPath+filename, nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("Expected 404, got %d", w.Code)
	}
}

func TestDisplayNotFound(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()

	req, err := http.NewRequest("GET", "/"+filename, nil)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatalf("Expected 404, got %d", w.Code)
	}
}

func TestPostCodeUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()
	extension := "txt"

	form := url.Values{}
	form.Add("content", "File content")
	form.Add("filename", filename)
	form.Add("extension", extension)

	req, err := http.NewRequest("POST", "/upload/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.PostForm = form
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", Config.siteURL)

	mux.ServeHTTP(w, req)

	if w.Code != 303 {
		t.Fatalf("Status code is not 303, but %d", w.Code)
	}

	req, err = http.NewRequest("GET", w.Header().Get("Location"), nil)
	req.Header.Set("User-Agent", "wget")
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatal("Status code is not 200, but " + strconv.Itoa(w.Code))
	}

	if w.Header().Get("Content-Disposition") != "attachment; filename=\""+filename+"."+extension+"\"" {
		t.Fatalf("Invalid original name %s, expected %s", w.Header().Get("Content-Disposition"), filename)
	}
}

func TestPostCodeUploadWhitelistedHeader(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()
	extension := "txt"

	form := url.Values{}
	form.Add("content", "File content")
	form.Add("filename", filename)
	form.Add("extension", extension)

	req, err := http.NewRequest("POST", "/upload/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.PostForm = form
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Linx-Expiry", "0")

	mux.ServeHTTP(w, req)

	if w.Code != 303 {
		t.Fatalf("Status code is not 303, but %d", w.Code)
	}
}

func TestPostCodeUploadNoReferrer(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()
	extension := "txt"

	form := url.Values{}
	form.Add("content", "File content")
	form.Add("filename", filename)
	form.Add("extension", extension)

	req, err := http.NewRequest("POST", "/upload/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.PostForm = form
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("Status code is not 400, but %d", w.Code)
	}
}

func TestPostCodeUploadBadOrigin(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()
	extension := "txt"

	form := url.Values{}
	form.Add("content", "File content")
	form.Add("filename", filename)
	form.Add("extension", extension)

	req, err := http.NewRequest("POST", "/upload/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.PostForm = form
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", Config.siteURL)
	req.Header.Set("Origin", "http://example.com")

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("Status code is not 400, but %d", w.Code)
	}
}

func TestPostCodeExpiryJSONUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	form := url.Values{}
	form.Add("content", "File content")
	form.Add("filename", "")
	form.Add("expires", "60")

	req, err := http.NewRequest("POST", "/upload/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.PostForm = form
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", Config.siteURL)
	req.Header.Set("Origin", strings.TrimSuffix(Config.siteURL, "/"))

	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 200, but %d", w.Code)
	}

	var myjson RespOkJSON
	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	myExp, err := strconv.ParseInt(myjson.Expiry, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	curTime := time.Now().Unix()

	if myExp < curTime {
		t.Fatalf("File expiry (%d) is smaller than current time (%d)", myExp, curTime)
	}

	if myjson.Size != "12" {
		t.Fatalf("File size was not 12 but %s", myjson.Size)
	}
}

func TestPostUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".txt"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}

	fw.Write([]byte("File content"))
	mw.Close()

	req, err := http.NewRequest("POST", "/upload/", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Referer", Config.siteURL)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 303 {
		t.Fatalf("Status code is not 303, but %d", w.Code)
	}
}

func TestPostJSONUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".txt"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}

	fw.Write([]byte("File content"))
	mw.Close()

	req, err := http.NewRequest("POST", "/upload/", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", Config.siteURL)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 200, but %d", w.Code)
	}

	var myjson RespOkJSON
	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	if myjson.Original_Name != filename {
		t.Fatalf("Filename is not '%s' but '%s' ", filename, myjson.Original_Name)
	}

	if myjson.Expiry != "0" {
		t.Fatalf("File expiry is not 0 but %s", myjson.Expiry)
	}

	if myjson.Size != "12" {
		t.Fatalf("File size was not 12 but %s", myjson.Size)
	}
}

func TestPostJSONUploadMaxExpiry(t *testing.T) {
	mux := setup()
	Config.maxExpiry = 300

	// include 0 to test edge case
	// https://github.com/andreimarcu/linx-server/issues/111
	testExpiries := []string{"86400", "-150", "0"}
	for _, expiry := range testExpiries {
		w := httptest.NewRecorder()

		filename := generateBarename() + ".txt"

		var b bytes.Buffer
		mw := multipart.NewWriter(&b)
		fw, err := mw.CreateFormFile("file", filename)
		if err != nil {
			t.Fatal(err)
		}

		fw.Write([]byte("File content"))
		mw.Close()

		req, err := http.NewRequest("POST", "/upload/", &b)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Linx-Expiry", expiry)
		if err != nil {
			t.Fatal(err)
		}

		mux.ServeHTTP(w, req)

		if w.Code != 200 {
			t.Log(w.Body.String())
			t.Fatalf("Status code is not 200, but %d", w.Code)
		}

		var myjson RespOkJSON
		err = json.Unmarshal([]byte(w.Body.String()), &myjson)
		if err != nil {
			t.Fatal(err)
		}

		myExp, err := strconv.ParseInt(myjson.Expiry, 10, 64)
		if err != nil {
			t.Fatal(err)
		}

		expected := time.Now().Add(time.Duration(Config.maxExpiry) * time.Second).Unix()
		if myExp != expected {
			t.Fatalf("File expiry is not %d but %s", expected, myjson.Expiry)
		}
	}

	Config.maxExpiry = 0
}

func TestPostExpiresJSONUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".txt"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("File content"))

	exp, err := mw.CreateFormField("expires")
	if err != nil {
		t.Fatal(err)
	}
	exp.Write([]byte("60"))

	mw.Close()

	req, err := http.NewRequest("POST", "/upload/", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", Config.siteURL)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 200, but %d", w.Code)
	}

	var myjson RespOkJSON
	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	if myjson.Original_Name != filename {
		t.Fatalf("Filename is not '%s' but '%s' ", filename, myjson.Original_Name)
	}

	myExp, err := strconv.ParseInt(myjson.Expiry, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	curTime := time.Now().Unix()

	if myExp < curTime {
		t.Fatalf("File expiry (%d) is smaller than current time (%d)", myExp, curTime)
	}

	if myjson.Size != "12" {
		t.Fatalf("File size was not 12 but %s", myjson.Size)
	}
}

func TestPostEmptyUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".txt"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}

	fw.Write([]byte(""))
	mw.Close()

	req, err := http.NewRequest("POST", "/upload/", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Referer", Config.siteURL)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 400, but %d", w.Code)
	}
}

func TestPostTooLargeUpload(t *testing.T) {
	mux := setup()
	oldMaxSize := Config.maxSize
	Config.maxSize = 2
	w := httptest.NewRecorder()

	filename := generateBarename() + ".txt"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}

	fw.Write([]byte("test content"))
	mw.Close()

	req, err := http.NewRequest("POST", "/upload/", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Referer", Config.siteURL)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 400, but %d", w.Code)
	}

	Config.maxSize = oldMaxSize
}

func TestPostEmptyJSONUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".txt"

	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}

	fw.Write([]byte(""))
	mw.Close()

	req, err := http.NewRequest("POST", "/upload/", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", Config.siteURL)
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 400, but %d", w.Code)
	}

	var myjson RespErrJSON
	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	if myjson.Error != "Empty file" {
		t.Fatal("Json 'error' was not 'Empty file' but " + myjson.Error)
	}
}

func TestPutUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".file"

	req, err := http.NewRequest("PUT", "/upload/"+filename, strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if strings.HasPrefix(w.Body.String(), Config.siteURL+"/") {
		t.Fatal("Response was not expected URL")
	}
}

func TestPutNoExtensionUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename()

	req, err := http.NewRequest("PUT", "/upload/"+filename, strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Body.String() == Config.siteURL+filename {
		t.Fatal("Filename was not random")
	}
}

func TestPutEmptyUpload(t *testing.T) {
	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".file"

	req, err := http.NewRequest("PUT", "/upload/"+filename, strings.NewReader(""))
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 400 {
		t.Fatalf("Status code is not 400, but %d", w.Code)
	}
}

func TestPutTooLargeUpload(t *testing.T) {
	mux := setup()
	oldMaxSize := Config.maxSize
	Config.maxSize = 2

	w := httptest.NewRecorder()

	filename := generateBarename() + ".file"

	req, err := http.NewRequest("PUT", "/upload/"+filename, strings.NewReader("File too big"))
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if w.Code != 500 {
		t.Log(w.Body.String())
		t.Fatalf("Status code is not 500, but %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "request body too large") {
		t.Fatal("Response did not contain 'request body too large'")
	}

	Config.maxSize = oldMaxSize
}

func TestPutJSONUpload(t *testing.T) {
	var myjson RespOkJSON

	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".file"

	req, err := http.NewRequest("PUT", "/upload/"+filename, strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Accept", "application/json")

	mux.ServeHTTP(w, req)

	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	if myjson.Original_Name != filename {
		t.Fatal("Filename was not provided one but " + myjson.Original_Name)
	}
}

func TestPutExpireJSONUpload(t *testing.T) {
	var myjson RespOkJSON

	mux := setup()
	w := httptest.NewRecorder()

	filename := generateBarename() + ".file"

	req, err := http.NewRequest("PUT", "/upload/"+filename, strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Linx-Expiry", "600")

	mux.ServeHTTP(w, req)

	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	expiry, err := strconv.Atoi(myjson.Expiry)
	if err != nil {
		t.Fatal("Expiry was not an integer")
	}
	if expiry < 1 {
		t.Fatal("Expiry was not set")
	}
}

func TestPutAndDelete(t *testing.T) {
	var myjson RespOkJSON

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("PUT", "/upload", strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Accept", "application/json")

	mux.ServeHTTP(w, req)

	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	// Delete it
	w = httptest.NewRecorder()
	req, err = http.NewRequest("DELETE", "/"+myjson.Filename, nil)
	req.Header.Set("Linx-Delete-Key", myjson.Delete_Key)
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatal("Status code was not 200, but " + strconv.Itoa(w.Code))
	}

	// Make sure it's actually gone
	w = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/"+myjson.Filename, nil)
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatal("Status code was not 404, but " + strconv.Itoa(w.Code))
	}
}

func TestPutAndSpecificDelete(t *testing.T) {
	var myjson RespOkJSON

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("PUT", "/upload", strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Linx-Delete-Key", "supersecret")

	mux.ServeHTTP(w, req)

	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	// Delete it
	w = httptest.NewRecorder()
	req, err = http.NewRequest("DELETE", "/"+myjson.Filename, nil)
	req.Header.Set("Linx-Delete-Key", "supersecret")
	mux.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatal("Status code was not 200, but " + strconv.Itoa(w.Code))
	}

	// Make sure it's actually gone
	w = httptest.NewRecorder()
	req, err = http.NewRequest("GET", "/"+myjson.Filename, nil)
	mux.ServeHTTP(w, req)

	if w.Code != 404 {
		t.Fatal("Status code was not 404, but " + strconv.Itoa(w.Code))
	}
}

func TestExtension(t *testing.T) {
	barename, extension := barePlusExt("test.jpg.gz")
	if barename != "testjpg" {
		t.Fatal("Barename was not testjpg, but " + barename)
	}
	if extension != "gz" {
		t.Fatal("Extension was not gz, but " + extension)
	}

	barename, extension = barePlusExt("test.tar.gz")
	if barename != "test" {
		t.Fatal("Barename was not test, but " + barename)
	}
	if extension != "tar.gz" {
		t.Fatal("Extension was not tar.gz, but " + extension)
	}
}

func TestInferSiteURL(t *testing.T) {
	oldSiteURL := Config.siteURL
	oldSitePath := Config.sitePath
	Config.siteURL = ""
	Config.sitePath = "/linxtest/"

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/API/", nil)
	req.Host = "example.com:8080"
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "http://example.com:8080/upload/") {
		t.Fatal("Site URL not found properly embedded in response")
	}

	Config.siteURL = oldSiteURL
	Config.sitePath = oldSitePath
}

func TestInferSiteURLProxied(t *testing.T) {
	oldSiteURL := Config.siteURL
	Config.siteURL = ""

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/API/", nil)
	req.Header.Add("X-Forwarded-Proto", "https")
	req.Host = "example.com:8080"
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "https://example.com:8080/upload/") {
		t.Fatal("Site URL not found properly embedded in response")
	}

	Config.siteURL = oldSiteURL
}

func TestInferSiteURLHTTPS(t *testing.T) {
	oldSiteURL := Config.siteURL
	oldCertFile := Config.certFile
	Config.siteURL = ""
	Config.certFile = "/dev/null"

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/API/", nil)
	req.Host = "example.com"
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "https://example.com/upload/") {
		t.Fatal("Site URL not found properly embedded in response")
	}

	Config.siteURL = oldSiteURL
	Config.certFile = oldCertFile
}

func TestInferSiteURLHTTPSFastCGI(t *testing.T) {
	oldSiteURL := Config.siteURL
	Config.siteURL = ""

	mux := setup()
	w := httptest.NewRecorder()

	req, err := http.NewRequest("GET", "/API/", nil)
	req.Host = "example.com"
	req.TLS = &tls.ConnectionState{HandshakeComplete: true}
	if err != nil {
		t.Fatal(err)
	}

	mux.ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "https://example.com/upload/") {
		t.Fatal("Site URL not found properly embedded in response")
	}

	Config.siteURL = oldSiteURL
}

func TestShutdown(t *testing.T) {
	os.RemoveAll(Config.filesDir)
	os.RemoveAll(Config.metaDir)
}

func TestPutAndGetCLI(t *testing.T) {
	var myjson RespOkJSON
	mux := setup()

	// upload file
	w := httptest.NewRecorder()
	req, err := http.NewRequest("PUT", "/upload", strings.NewReader("File content"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept", "application/json")
	mux.ServeHTTP(w, req)

	err = json.Unmarshal([]byte(w.Body.String()), &myjson)
	if err != nil {
		t.Fatal(err)
	}

	// request file without wget user agent
	w = httptest.NewRecorder()
	req, err = http.NewRequest("GET", myjson.Url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36")
	mux.ServeHTTP(w, req)

	contentType := w.Header().Get("Content-Type")
	if strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Didn't receive file display page but %s", contentType)
	}

	// request file with wget user agent
	w = httptest.NewRecorder()
	req, err = http.NewRequest("GET", myjson.Url, nil)
	req.Header.Set("User-Agent", "wget")
	if err != nil {
		t.Fatal(err)
	}
	mux.ServeHTTP(w, req)

	contentType = w.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Didn't receive file directly but %s", contentType)
	}

}
