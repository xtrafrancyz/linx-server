package s3

import (
	"context"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/andreimarcu/linx-server/backends"
	"github.com/andreimarcu/linx-server/helpers"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type S3Backend struct {
	bucket string
	svc    *s3.Client
}

func (b S3Backend) Delete(key string) error {
	_, err := b.svc.DeleteObject(context.TODO(), &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return err
	}
	return nil
}

func (b S3Backend) Exists(key string) (bool, error) {
	_, err := b.svc.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	return err == nil, err
}

func (b S3Backend) Head(key string) (metadata backends.Metadata, err error) {
	var result *s3.HeadObjectOutput
	result, err = b.svc.HeadObject(context.TODO(), &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var nf *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &nf) {
			err = backends.NotFoundErr
		}
		return
	}

	metadata, err = unmapMetadata(result.Metadata)
	return
}

func (b S3Backend) Get(key string) (metadata backends.Metadata, r io.ReadCloser, err error) {
	var result *s3.GetObjectOutput
	result, err = b.svc.GetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		var nf *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &nf) {
			err = backends.NotFoundErr
		}
		return
	}

	metadata, err = unmapMetadata(result.Metadata)
	r = result.Body
	return
}

func (b S3Backend) ServeFile(key string, w http.ResponseWriter, r *http.Request) (err error) {
	var result *s3.GetObjectOutput

	if r.Header.Get("Range") != "" {
		result, err = b.svc.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(key),
			Range:  aws.String(r.Header.Get("Range")),
		})

		w.WriteHeader(206)
		w.Header().Set("Content-Range", *result.ContentRange)
		w.Header().Set("Content-Length", strconv.FormatInt(*result.ContentLength, 10))
		w.Header().Set("Accept-Ranges", "bytes")

	} else {
		result, err = b.svc.GetObject(context.TODO(), &s3.GetObjectInput{
			Bucket: aws.String(b.bucket),
			Key:    aws.String(key),
		})

	}

	if err != nil {
		var nsk *types.NoSuchKey
		var nf *types.NotFound
		if errors.As(err, &nsk) || errors.As(err, &nf) {
			err = backends.NotFoundErr
		}
		return
	}

	_, err = io.Copy(w, result.Body)

	return
}

func mapMetadata(m backends.Metadata) map[string]string {
	return map[string]string{
		"OriginalName": m.OriginalName,
		"Expiry":       strconv.FormatInt(m.Expiry.Unix(), 10),
		"Deletekey":    m.DeleteKey,
		"Size":         strconv.FormatInt(m.Size, 10),
		"Mimetype":     m.Mimetype,
		"Sha256sum":    m.Sha256sum,
		"AccessKey":    m.AccessKey,
	}
}

func unmapMetadata(input map[string]string) (m backends.Metadata, err error) {
	expiry, err := strconv.ParseInt(input["Expiry"], 10, 64)
	if err != nil {
		return m, err
	}
	m.Expiry = time.Unix(expiry, 0)

	m.Size, err = strconv.ParseInt(input["Size"], 10, 64)
	if err != nil {
		return
	}

	m.DeleteKey = input["Deletekey"]
	if m.DeleteKey == "" {
		m.DeleteKey = input["Delete_key"]
	}

	m.OriginalName = input["OriginalName"]
	m.Mimetype = input["Mimetype"]
	m.Sha256sum = input["Sha256sum"]

	if key, ok := input["AccessKey"]; ok {
		m.AccessKey = key
	}
	return
}

func (b S3Backend) Put(key, originalName string, r io.Reader, expiry time.Time, deleteKey, accessKey string) (m backends.Metadata, err error) {
	tmpDst, err := ioutil.TempFile("", "linx-server-upload")
	if err != nil {
		return m, err
	}
	defer tmpDst.Close()
	defer os.Remove(tmpDst.Name())

	bytes, err := io.Copy(tmpDst, r)
	if bytes == 0 {
		return m, backends.FileEmptyError
	} else if err != nil {
		return m, err
	}

	_, err = tmpDst.Seek(0, 0)
	if err != nil {
		return m, err
	}

	m, err = helpers.GenerateMetadata(tmpDst)
	if err != nil {
		return
	}
	m.OriginalName = originalName
	m.Expiry = expiry
	m.DeleteKey = deleteKey
	m.AccessKey = accessKey
	// XXX: we may not be able to write this to AWS easily
	// m.ArchiveFiles, _ = helpers.ListArchiveFiles(m.Mimetype, m.Size, tmpDst)

	_, err = tmpDst.Seek(0, 0)
	if err != nil {
		return m, err
	}

	uploader := manager.NewUploader(b.svc)
	input := &s3.PutObjectInput{
		Bucket:   aws.String(b.bucket),
		Key:      aws.String(key),
		Body:     tmpDst,
		Metadata: mapMetadata(m),
	}
	_, err = uploader.Upload(context.TODO(), input)
	if err != nil {
		return
	}

	return
}

func (b S3Backend) PutMetadata(key string, m backends.Metadata) (err error) {
	_, err = b.svc.CopyObject(context.TODO(), &s3.CopyObjectInput{
		Bucket:            aws.String(b.bucket),
		Key:               aws.String(key),
		CopySource:        aws.String("/" + b.bucket + "/" + key),
		Metadata:          mapMetadata(m),
		MetadataDirective: types.MetadataDirectiveReplace,
	})
	if err != nil {
		return
	}

	return
}

func (b S3Backend) Size(key string) (int64, error) {
	input := &s3.HeadObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(key),
	}
	result, err := b.svc.HeadObject(context.TODO(), input)
	if err != nil {
		return 0, err
	}

	return *result.ContentLength, nil
}

func (b S3Backend) List() ([]string, error) {
	var output []string
	input := &s3.ListObjectsInput{
		Bucket: aws.String(b.bucket),
	}

	results, err := b.svc.ListObjects(context.TODO(), input)
	if err != nil {
		return nil, err
	}

	for _, object := range results.Contents {
		output = append(output, *object.Key)
	}

	return output, nil
}

func NewS3Backend(bucket string, region string, endpoint string, forcePathStyle bool) S3Backend {
	ctx := context.TODO()

	// Load default config
	cfg, err := config.LoadDefaultConfig(ctx, func(opts *config.LoadOptions) error {
		if region != "" {
			opts.Region = region
		}
		return nil
	})

	if err != nil {
		panic(err)
	}

	// Create S3 client with optional customizations
	svc := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		if forcePathStyle {
			o.UsePathStyle = true
		}
	})

	return S3Backend{bucket: bucket, svc: svc}
}
