package helpers

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"io"
	"sort"

	"github.com/nwaples/rardecode"
)

type ReadSeekerAt interface {
	io.Reader
	io.Seeker
	io.ReaderAt
}

func ListArchiveFiles(mimetype string, size int64, r ReadSeekerAt) (files []string, err error) {
	switch mimetype {
	case "application/x-tar":
		files, err = listTarFiles(r)
	case "application/gzip", "application/x-gzip":
		gzf, err0 := gzip.NewReader(r)
		if err0 != nil {
			return nil, err0
		}
		files, err = listTarFiles(gzf)
	case "application/x-bzip", "application/bzip2", "application/x-bzip2":
		files, err = listTarFiles(bzip2.NewReader(r))
	case "application/zip", "application/x-zip", "application/x-zip-compressed":
		zf, err := zip.NewReader(r, size)
		if err != nil {
			return nil, err
		}
		for _, f := range zf.File {
			files = append(files, f.Name)
		}
	case "application/x-rar", "application/x-rar-compressed":
		reader, err := rardecode.NewReader(r, "")
		if err != nil {
			return nil, err
		}
		for {
			next, err := reader.Next()
			if err != nil {
				if err == io.EOF {
					break
				}
				return nil, err
			}
			if next.IsDir {
				files = append(files, next.Name+"/")
			} else {
				files = append(files, next.Name)
			}
		}
	}

	if len(files) > 0 {
		sort.Strings(files)
	}
	return
}

func listTarFiles(input io.Reader) (files []string, err error) {
	reader := tar.NewReader(input)
	for {
		hdr, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag == tar.TypeDir || hdr.Typeflag == tar.TypeReg {
			files = append(files, hdr.Name)
		}
	}
	return files, nil
}
