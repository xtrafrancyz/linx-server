package localfs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/andreimarcu/linx-server/helpers"
	"github.com/shirou/gopsutil/v4/disk"
)

type LocalfsBackend struct {
	metaPath       string
	filesPath      string
	minFreeSpaceGB float64
}

type MetadataJSON struct {
	OriginalName string   `json:"original_name"`
	DeleteKey    string   `json:"delete_key"`
	AccessKey    string   `json:"access_key,omitempty"`
	Sha256sum    string   `json:"sha256sum"`
	Mimetype     string   `json:"mimetype"`
	Size         int64    `json:"size"`
	Expiry       int64    `json:"expiry"`
	ArchiveFiles []string `json:"archive_files,omitempty"`
}

func (b LocalfsBackend) Delete(key string) error {
	return errors.Join(
		os.Remove(path.Join(b.filesPath, key)),
		os.Remove(path.Join(b.metaPath, key)),
	)
}

func (b LocalfsBackend) Exists(key string) (bool, error) {
	_, err := os.Stat(path.Join(b.filesPath, key))
	if err == nil {
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else {
		return false, err
	}
}

func (b LocalfsBackend) Head(key string) (metadata backends.Metadata, err error) {
	f, err := os.Open(path.Join(b.metaPath, key))
	if os.IsNotExist(err) {
		return metadata, backends.NotFoundErr
	} else if err != nil {
		return metadata, backends.BadMetadata
	}
	defer f.Close()

	decoder := json.NewDecoder(f)

	mjson := MetadataJSON{}
	if err := decoder.Decode(&mjson); err != nil {
		return metadata, backends.BadMetadata
	}

	metadata.OriginalName = mjson.OriginalName
	metadata.DeleteKey = mjson.DeleteKey
	metadata.AccessKey = mjson.AccessKey
	metadata.Mimetype = mjson.Mimetype
	metadata.ArchiveFiles = mjson.ArchiveFiles
	metadata.Sha256sum = mjson.Sha256sum
	metadata.Expiry = time.Unix(mjson.Expiry, 0)
	metadata.Size = mjson.Size

	return
}

func (b LocalfsBackend) Get(key string) (metadata backends.Metadata, f io.ReadCloser, err error) {
	metadata, err = b.Head(key)
	if err != nil {
		return
	}

	f, err = os.Open(path.Join(b.filesPath, key))
	if err != nil {
		return
	}

	return
}

func (b LocalfsBackend) ServeFile(key string, w http.ResponseWriter, r *http.Request) (err error) {
	_, err = b.Head(key)
	if err != nil {
		return
	}

	filePath := path.Join(b.filesPath, key)
	http.ServeFile(w, r, filePath)

	return
}

func (b LocalfsBackend) writeMetadata(key string, metadata backends.Metadata) error {
	metaPath := path.Join(b.metaPath, key)

	mjson := MetadataJSON{
		OriginalName: metadata.OriginalName,
		DeleteKey:    metadata.DeleteKey,
		AccessKey:    metadata.AccessKey,
		Mimetype:     metadata.Mimetype,
		ArchiveFiles: metadata.ArchiveFiles,
		Sha256sum:    metadata.Sha256sum,
		Expiry:       metadata.Expiry.Unix(),
		Size:         metadata.Size,
	}

	dst, err := os.Create(metaPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	encoder := json.NewEncoder(dst)
	err = encoder.Encode(mjson)
	if err != nil {
		os.Remove(metaPath)
		return err
	}

	return nil
}

func (b LocalfsBackend) Put(key, originalName string, r io.Reader, expiry time.Time, deleteKey, accessKey string) (m backends.Metadata, err error) {
	var cachedUsage *disk.UsageStat
	minFreeBytes := uint64(b.minFreeSpaceGB * 1024 * 1024 * 1024)
	if b.minFreeSpaceGB > 0 {
		cachedUsage, err = disk.Usage(b.filesPath)
		if err != nil {
			return m, fmt.Errorf("failed to check disk usage: %w", err)
		}

		if cachedUsage.Free < minFreeBytes {
			return m, fmt.Errorf("insufficient disk space: %.2f GB free, minimum required is %.2f GB",
				float64(cachedUsage.Free)/(1024*1024*1024), b.minFreeSpaceGB)
		}
	}

	filePath := path.Join(b.filesPath, key)

	dst, err := os.Create(filePath)
	if err != nil {
		return
	}
	defer dst.Close()

	bytes, err := io.Copy(dst, r)
	if bytes == 0 {
		os.Remove(filePath)
		return m, backends.FileEmptyError
	} else if err != nil {
		os.Remove(filePath)
		return m, err
	}

	if b.minFreeSpaceGB > 0 {
		freeAfterUpload := cachedUsage.Free - uint64(bytes)
		if freeAfterUpload < minFreeBytes {
			os.Remove(filePath)
			return m, fmt.Errorf("insufficient disk space: would have %.2f GB free after upload, minimum required is %.2f GB",
				float64(freeAfterUpload)/(1024*1024*1024), b.minFreeSpaceGB)
		}
	}

	dst.Seek(0, 0)
	m, err = helpers.GenerateMetadata(dst)
	if err != nil {
		os.Remove(filePath)
		return
	}
	dst.Seek(0, 0)

	m.OriginalName = originalName
	m.Expiry = expiry
	m.DeleteKey = deleteKey
	m.AccessKey = accessKey
	m.ArchiveFiles, _ = helpers.ListArchiveFiles(m.Mimetype, m.Size, dst)

	err = b.writeMetadata(key, m)
	if err != nil {
		os.Remove(filePath)
		return
	}

	return
}

func (b LocalfsBackend) PutMetadata(key string, m backends.Metadata) (err error) {
	err = b.writeMetadata(key, m)
	if err != nil {
		return
	}

	return
}

func (b LocalfsBackend) Size(key string) (int64, error) {
	fileInfo, err := os.Stat(path.Join(b.filesPath, key))
	if err != nil {
		return 0, err
	}

	return fileInfo.Size(), nil
}

func (b LocalfsBackend) List() ([]string, error) {
	var output []string

	files, err := os.ReadDir(b.filesPath)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		output = append(output, file.Name())
	}

	return output, nil
}

func NewLocalfsBackend(metaPath string, filesPath string, minFreeSpaceGB float64) LocalfsBackend {
	return LocalfsBackend{
		metaPath:       metaPath,
		filesPath:      filesPath,
		minFreeSpaceGB: minFreeSpaceGB,
	}
}
