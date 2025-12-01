package gitlabgoproxy

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/goproxy/goproxy"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type (
	S3Config struct {
		AccessKeyID     string `json:"access_key_id" toml:"access_key_id" yaml:"access_key_id"`
		SecretAccessKey string `json:"secret_access_key" toml:"secret_access_key" yaml:"secret_access_key"`
		Endpoint        string `json:"endpoint" toml:"endpoint" yaml:"endpoint"`
		DisableTLS      bool   `json:"disable_tls" toml:"disable_tls" yaml:"disable_tls"`
		Bucket          string `json:"bucket" toml:"bucket" yaml:"bucket"`
		Enable          bool   `json:"enable" toml:"enable" yaml:"enable"`
	}

	S3Cache struct {
		client *minio.Client
		bucket string
	}
)

var _ goproxy.Cacher = (*S3Cache)(nil)

const partSize = uint64(100 << 20)

func NewS3Cache(conf S3Config) (goproxy.Cacher, error) {
	if !conf.Enable {
		return nil, errors.New("unload cacher")
	}

	opts := &minio.Options{
		Creds:        credentials.NewStaticV4(conf.AccessKeyID, conf.SecretAccessKey, ""),
		Secure:       !conf.DisableTLS,
		BucketLookup: minio.BucketLookupPath,
	}
	client, err := minio.New(conf.Endpoint, opts)
	if err != nil {
		return nil, err
	}
	return &S3Cache{client: client, bucket: conf.Bucket}, nil
}

func (s3 *S3Cache) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	o, err := s3.client.GetObject(ctx, s3.bucket, name, minio.GetObjectOptions{})
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	oi, err := o.Stat()
	if err != nil {
		if minio.ToErrorResponse(err).StatusCode == http.StatusNotFound {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	return newS3Cache(o, oi), nil
}

func (s3 *S3Cache) Put(ctx context.Context, name string, content io.ReadSeeker) error {
	size, err := content.Seek(0, io.SeekEnd)
	if err != nil {
		return err
	}
	if _, err := content.Seek(0, io.SeekStart); err != nil {
		return err
	}

	contentType := "application/octet-stream"
	nameExt := filepath.Ext(name)
	switch {
	case nameExt == ".info", strings.HasSuffix(name, "/@latest"):
		contentType = "application/json; charset=utf-8"
	case nameExt == ".mod", strings.HasSuffix(name, "/@v/list"):
		contentType = "text/plain; charset=utf-8"
	case nameExt == ".zip":
		contentType = "application/zip"
	case strings.HasPrefix(name, "sumdb/"):
		if elems := strings.Split(name, "/"); len(elems) >= 3 {
			switch elems[2] {
			case "latest", "lookup":
				contentType = "text/plain; charset=utf-8"
			}
		}
	}

	_, err = s3.client.PutObject(ctx, s3.bucket, name, content, size, minio.PutObjectOptions{
		ContentType:    contentType,
		PartSize:       partSize,
		SendContentMd5: true,
	})
	return err
}

// s3Cache is the cache returned by [s3Cacher.Get].
type s3Cache struct {
	*minio.Object
	minio.ObjectInfo
}

// newS3Cache creates a new [s3Cache].
func newS3Cache(o *minio.Object, oi minio.ObjectInfo) *s3Cache {
	return &s3Cache{o, oi}
}

// LastModified implements [github.com/goproxy/goproxy.Cacher.Get].
func (s3c *s3Cache) LastModified() time.Time {
	return s3c.ObjectInfo.LastModified
}

// ETag implements [github.com/goproxy/goproxy.Cacher.Get].
func (s3c *s3Cache) ETag() string {
	if s3c.ObjectInfo.ETag != "" {
		return strconv.Quote(s3c.ObjectInfo.ETag)
	}
	return ""
}
