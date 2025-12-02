package gitlabgoproxy

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	az "archive/zip"
)

type SmartFile struct {
	*os.File
	Ctx    context.Context
	closed int32
}

var _ io.ReadSeekCloser = (*SmartFile)(nil)

func Save(ctx context.Context, input io.Reader) (io.ReadSeekCloser, int64, error) {
	file, err := os.CreateTemp(os.TempDir(), "gitlab-*")
	if err != nil {
		return nil, 0, err
	}
	size, err := io.Copy(file, input)
	if err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return nil, size, err
	}

	f2, err := os.Open(file.Name())
	if err != nil {
		return nil, 0, err
	}
	cf := &SmartFile{File: f2, Ctx: ctx}
	go cf.run()
	return cf, size, nil
}

func Create(ctx context.Context) (*SmartFile, error) {
	file, err := os.CreateTemp(os.TempDir(), "gitlab-*")
	if err != nil {
		return nil, err
	}
	cf := &SmartFile{File: file, Ctx: ctx}
	go cf.run()
	return cf, nil
}

func (cf *SmartFile) Close() error {
	defer func() {
		if atomic.CompareAndSwapInt32(&cf.closed, 0, 1) {
			os.Remove(cf.File.Name())
		}
	}()
	err := cf.File.Close()
	return err
}

func (cf *SmartFile) run() {
	slog.Info("this file will be automatically cleaned up later.", slog.String("file", cf.Name()))
	<-cf.Ctx.Done()
	slog.Info("automatically deleted this file", slog.String("file", cf.Name()), slog.String("reason", cf.Ctx.Err().Error()))
	cf.Close()
}

func UnzipArchiveFromGitlab(workspace string, depth int, archive string) error {
	reader, err := az.OpenReader(archive)
	if err != nil {
		return err
	}
	defer reader.Close()

	segmentsToSkip := 1 + depth

	for _, file := range reader.File {
		relPath := file.Name
		for i := 0; i < segmentsToSkip; i++ {
			idx := strings.IndexByte(relPath, '/')
			if idx == -1 {
				relPath = ""
				break
			}
			relPath = relPath[idx+1:]
		}

		if relPath == "" {
			continue
		}

		fp := filepath.Join(workspace, relPath)
		if !strings.HasPrefix(fp, filepath.Clean(workspace)+string(os.PathSeparator)) {
			continue
		}

		if file.FileInfo().IsDir() {
			if err := os.MkdirAll(fp, os.ModePerm); err != nil {
				return err
			}
			continue
		}

		if err = os.MkdirAll(filepath.Dir(fp), os.ModePerm); err != nil {
			return err
		}
		dst, err := os.Create(fp)
		if err != nil {
			return err
		}
		src, err := file.Open()
		if err != nil {
			dst.Close()
			return err
		}
		_, err = io.Copy(dst, src)
		src.Close()
		dst.Close()
		if err != nil {
			return err
		}
	}
	return nil
}
