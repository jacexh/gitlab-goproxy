package gitlabgoproxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/go-jimu/components/sloghelper"
	"github.com/goproxy/goproxy"
	"golang.org/x/mod/module"
	"golang.org/x/mod/zip"
)

type (
	GitlabFetcher struct {
		gitlab GitLab
		config GitlabFetcherConfig
	}

	Info struct {
		Version string
		Time    time.Time // commit time
	}

	Locator struct {
		Repository string
		SubPath    string
		Ref        string
	}

	GitLab interface {
		ListTags(ctx context.Context, repository string, prefix string) ([]*Info, error)
		GetTag(ctx context.Context, repository, tag string) (*Info, error)
		GetFile(ctx context.Context, repository, path, ref string) ([]byte, error)
		Download(ctx context.Context, repository, dir, ref string) (io.Reader, error) // https://go.dev/ref/mod#zip-files, TODO: 主module的zip文件里不包含任何子module，子module的zip中只包含自身文件
		IsProject(context.Context, string) (bool, error)
	}

	GitlabFetcherConfig struct {
		Endpoint    string `json:"endpoint" yaml:"endpoint" toml:"endpoint"`
		AccessToken string `json:"access_token" yaml:"access_token" toml:"access_token"`
		Mask        string `json:"mask" yaml:"mask" toml:"mask"`
	}

	UpstreamConfig struct {
		Proxy string `json:"proxy" yaml:"proxy" toml:"proxy"`
	}

	Config struct {
		Masks    []GitlabFetcherConfig `json:"masks" yaml:"masks" toml:"masks"`
		Upstream UpstreamConfig        `json:"upstream" yaml:"upstream" toml:"upstream"`
	}

	MixedFetcher struct {
		Masks    []*GitlabFetcher
		Upstream goproxy.Fetcher
	}
)

var (
	_       goproxy.Fetcher = (*GitlabFetcher)(nil)
	matcher                 = regexp.MustCompile(`^v[0-9]+$`)
)

func NewGitlabFetcher(conf GitlabFetcherConfig) goproxy.Fetcher {
	return &GitlabFetcher{gitlab: NewGitlabHost(conf), config: conf}
}

// Query:
// - gitlab.com/wongidle/foobar v0.1.1  -> wongidle/foobar v0.1.1
// - gitlab.com/wongidle/foobar/internal v0.1.1
//   - good: wongidle/foobar | internal | internal/v0.1.1
//   - bad: wongidle/foobar |
func (gf *GitlabFetcher) Query(ctx context.Context, path, query string) (string, time.Time, error) {
	slog.Info("calling Query function", slog.String("path", path), slog.String("query", query))
	if err := module.Check(path, query); err != nil {
		slog.Warn("bad path-query pair", slog.String("path", path), slog.String("query", query), slog.String("error", err.Error()))
		return "", time.Time{}, err
	}
	loc, err := gf.Extract(ctx, path, query)
	if err != nil {
		return "", time.Time{}, err
	}
	slog.Info("fetch tag from remote host", slog.String("path", path), slog.String("query", query))
	info, err := gf.gitlab.GetTag(ctx, loc.Repository, loc.Ref)
	if err != nil {
		slog.Warn("failed to get tag info from gitlab host", slog.String("project", path), slog.String("ref", query), sloghelper.Error(err))
		return "", time.Time{}, err
	}
	return info.Version, info.Time, nil
}

// Download ..
func (gf *GitlabFetcher) Download(ctx context.Context, path, version string) (info, mod, zip io.ReadSeekCloser, err error) {
	slog.Info("start to download", slog.String("path", path), slog.String("version", version))
	if err = module.Check(path, version); err != nil {
		slog.Warn("bad path-version pair", slog.String("path", path), slog.String("version", version), slog.String("error", err.Error()))
		return nil, nil, nil, err
	}
	loc, err := gf.Extract(ctx, path, version)
	if err != nil {
		return nil, nil, nil, err
	}

	var mu = &sync.Mutex{}

	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		var errInfo error
		info, errInfo = gf.SaveInfo(ctx, loc)
		if errInfo != nil {
			mu.Lock()
			defer mu.Unlock()
			err = fmt.Errorf("save info error: %w", errInfo)
			cancel()
		}
	}()

	go func() {
		defer wg.Done()
		var errMod error
		mod, errMod = gf.SaveGoMod(ctx, loc)
		if errMod != nil {
			mu.Lock()
			defer mu.Unlock()
			err = fmt.Errorf("save go.mod error: %w", errMod)
			cancel()
		}
	}()

	go func() {
		defer wg.Done()
		var errZip error
		zip, errZip = gf.Archive(ctx, loc, path, version)
		if errZip != nil {
			mu.Lock()
			defer mu.Unlock()
			err = fmt.Errorf("save archive file error: %w", errZip)
			cancel()
		}
	}()

	wg.Wait()
	if err != nil {
		return nil, nil, nil, err
	}
	return
}

func (gf *GitlabFetcher) SaveInfo(ctx context.Context, loc *Locator) (io.ReadSeekCloser, error) {
	info, err := gf.gitlab.GetTag(ctx, loc.Repository, loc.Ref)
	if err != nil {
		return nil, err
	}
	if loc.SubPath != "" {
		info.Version = loc.Ref[strings.LastIndex(loc.Ref, "/")+1:]
	}

	data, err := json.Marshal(info)
	if err != nil {
		return nil, err
	}
	r, _, err := Save(ctx, bytes.NewReader(data))
	return r, err
}

func (gf *GitlabFetcher) SaveGoMod(ctx context.Context, loc *Locator) (io.ReadSeekCloser, error) {
	data, err := gf.gitlab.GetFile(ctx, loc.Repository, filepath.Join(loc.SubPath, "go.mod"), loc.Ref)
	if err != nil {
		return nil, err
	}

	r, _, err := Save(ctx, bytes.NewReader(data))
	return r, err
}

func (gh *GitlabFetcher) Archive(ctx context.Context, loc *Locator, path, version string) (io.ReadSeekCloser, error) {
	dir, err := os.MkdirTemp(os.TempDir(), "gitlab-*")
	if err != nil {
		return nil, err
	}
	// defer os.RemoveAll(dir)

	reader, err := gh.gitlab.Download(ctx, loc.Repository, loc.SubPath, loc.Ref)
	if err != nil {
		return nil, err
	}
	filename := "archive.zip"
	fp := filepath.Join(dir, filename)
	f, err := os.Create(fp)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(f, reader)
	if err != nil {
		f.Close()
		return nil, err
	}
	f.Close()
	// 保存存档文件结束

	// 解压缩到 workspace目录下
	ws := filepath.Join(dir, "workspace")

	depth := 0
	if loc.SubPath != "" {
		depth = strings.Count(loc.SubPath, "/") + 1
	}
	err = UnzipArchiveFromGitlab(ws, depth, fp)
	if err != nil {
		return nil, err
	}

	// x/mod再处理
	sf, err := Create(ctx)
	if err != nil {
		return nil, err
	}
	slog.Info("output", slog.String("output", sf.Name()))
	if err = zip.CreateFromDir(sf, module.Version{Path: path, Version: version}, ws); err != nil {
		return nil, err
	}
	return sf, nil
}

func (gf *GitlabFetcher) Extract(ctx context.Context, path, query string) (*Locator, error) {
	if err := module.Check(path, query); err != nil {
		return nil, err
	}
	ps := strings.Split(path, "/") // ["gitlab.com", "wongidle", "mutiples", "pkg", "srv", "v2"]
	// 最简单的模式， host/group/proj  v0/1 版本，大多数情况
	loc := &Locator{Ref: query}

	tail := len(ps) - 1
	for cursor := 2; cursor <= tail; cursor++ {
		proj := strings.Join(ps[1:cursor+1], "/")
		ok, err := gf.gitlab.IsProject(ctx, proj)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		// ["gitlab.com", "wongidle", "foobar", "pkg"]
		loc.Repository = proj
		if cursor == tail {
			loc.Ref = query
			return loc, nil
		}
		if cursor < tail {
			if isV2 := matcher.MatchString(ps[tail]); isV2 {
				tail--
			}
			if cursor < tail {
				// github.com/foo/bar/xxxx/yyy v1.0.0   invalid version
				// github.com/foo/bar/v2 v2.0.0  foo/bar,  "", v2.0.0
				// github.com/foo/bar/echo/world v1.0.0  echo/world/v1.0.0, world/v1.0.0
				dirs := ps[cursor+1 : tail+1]
				// 从尾部开始递归
				for index := len(dirs); index > 0; index-- {
					subPath := strings.Join(dirs[0:index], "/")
					ref := subPath + "/" + query
					_, err = gf.gitlab.GetFile(ctx, loc.Repository, subPath+"/go.mod", ref)
					if err != nil {
						slog.Warn("no go.mod founded in subpath", slog.String("project", loc.Repository),
							slog.String("subpath", subPath), slog.String("version", ref), slog.String("error", err.Error()))
						continue
					}
					loc.SubPath = subPath
					loc.Ref = ref
					return loc, nil
				}
				return nil, errors.New("invalid module path")
			}
		}
		return loc, nil
	}
	return nil, fmt.Errorf("cannot found gitlab project with path=%s  query=%s", path, query)
}

// List 通过Git Repository的Tag列表返回查询结果
//
//	gitlab/wongidle/foobar -> [v0.1.0, v0.1.1]
//	gitlab/wongidle/foobar/internal -> error
//	gitlab/wongidle/foobar/pkg -> [v0.2.0, v0.2.1] -> [pkg/v0.2.0, pkg/v0.2.1]
func (gf *GitlabFetcher) List(ctx context.Context, path string) ([]string, error) {
	slog.Info("calling List function", slog.String("path", path))
	repo, subs, verPrefix, err := gf.ExtractSubPath(ctx, path)
	if err != nil {
		return nil, err
	}

	prefixs := make([]string, 0)
	switch {
	case verPrefix != "" && len(subs) > 0:
		// 尾部遍历
		for tail := len(subs) - 1; tail >= 0; tail-- {
			prefixs = append(prefixs, strings.Join(subs[:tail+1], "/")+"/"+verPrefix)
		}

	case verPrefix != "" && len(subs) == 0:
		prefixs = append(prefixs, verPrefix)

	case verPrefix == "" && len(subs) > 0:
		for tail := len(subs) - 1; tail >= 0; tail-- {
			prefixs = append(prefixs, strings.Join(subs[:tail+1], "/")+"/v")
		}

	case verPrefix == "" && len(subs) == 0:
		prefixs = append(prefixs, "v0.", "v1.")
	}

	ret := make([]string, 0)
	for _, prefix := range prefixs {
		tags, err := gf.gitlab.ListTags(ctx, repo, prefix)
		if err != nil {
			return nil, err
		}
		if len(tags) == 0 {
			continue
		}

		for _, tag := range tags {
			ret = append(ret, tag.Version[strings.LastIndex(tag.Version, "/")+1:])
		}
		// 遍历v0. 继续v1.
		if verPrefix == "" && len(subs) == 0 {
			continue
		}
		return ret, nil
	}
	if len(ret) > 0 {
		return ret, nil
	}
	return nil, errors.New("no matching versions")
}

func (gf *GitlabFetcher) ExtractSubPath(ctx context.Context, path string) (string, []string, string, error) {
	verPrefix := ""

	escaped, err := module.EscapePath(path)
	if err != nil {
		return "", nil, verPrefix, err
	}
	ps := strings.Split(escaped, "/")
	tail := len(ps) - 1

	for cursor := 2; cursor <= tail; cursor++ {
		proj := strings.Join(ps[1:cursor+1], "/")
		ok, err := gf.gitlab.IsProject(ctx, proj)
		if err != nil {
			return "", nil, verPrefix, err
		}
		if !ok {
			continue
		}

		if cursor < tail {
			if matcher.MatchString(ps[tail]) {
				verPrefix = ps[tail]
				tail--
			}
			if cursor < tail {
				return proj, ps[cursor+1 : tail+1], verPrefix, nil
			}
		}
		return proj, []string{}, verPrefix, nil
	}
	return "", nil, verPrefix, errors.New("no matching versions")
}

func (gf *GitlabFetcher) NeedFetch(path string) bool {
	return strings.HasPrefix(path, gf.config.Mask)
}

func NewMixedFetcher(conf Config) *MixedFetcher {
	mf := &MixedFetcher{}
	mf.Upstream = &goproxy.GoFetcher{Env: []string{fmt.Sprintf("GOPROXY=%s,direct", conf.Upstream.Proxy)}}
	for _, c := range conf.Masks {
		mf.Masks = append(mf.Masks, NewGitlabFetcher(c).(*GitlabFetcher))
	}
	return mf
}

func (mf *MixedFetcher) Download(ctx context.Context, path string, version string) (io.ReadSeekCloser, io.ReadSeekCloser, io.ReadSeekCloser, error) {
	for _, gf := range mf.Masks {
		if gf.NeedFetch(path) {
			return gf.Download(ctx, path, version)
		}
	}
	slog.Info("redirect download request to upstrea proxy", slog.String("path", path), slog.String("version", version))
	return mf.Upstream.Download(ctx, path, version)
}

func (mf *MixedFetcher) List(ctx context.Context, path string) ([]string, error) {
	for _, gf := range mf.Masks {
		if gf.NeedFetch(path) {
			return gf.List(ctx, path)
		}
	}
	slog.Info("redirect list request to upstrea proxy", slog.String("path", path))
	return mf.Upstream.List(ctx, path)
}

func (mf *MixedFetcher) Query(ctx context.Context, path string, query string) (string, time.Time, error) {
	for _, gf := range mf.Masks {
		if gf.NeedFetch(path) {
			return gf.Query(ctx, path, query)
		}
	}
	slog.Info("redirect query request to upstrea proxy", slog.String("path", path), slog.String("query", query))
	return mf.Upstream.Query(ctx, path, query)
}
