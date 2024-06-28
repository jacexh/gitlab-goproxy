package gitlabgoproxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/go-jimu/components/sloghelper"
	"github.com/goproxy/goproxy"
	"golang.org/x/mod/module"
)

type (
	GitlabFetcher struct {
		gitlab GitLab
	}

	GitlabFetcherConfig struct {
		BaseURL     string
		Mask        string
		AccessToken string
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
)

var (
	_       goproxy.Fetcher = (*GitlabFetcher)(nil)
	matcher                 = regexp.MustCompile(`^v[0-9]+$`)
)

func NewGitlabFetcher(conf GitlabFetcherConfig) goproxy.Fetcher {
	return &GitlabFetcher{gitlab: NewGitlabHost(conf)}
}

// Query:
// - gitlab.com/wongidle/foobar v0.1.1  -> wongidle/foobar v0.1.1
// - gitlab.com/wongidle/foobar/internal v0.1.1
//   - good: wongidle/foobar | internal | internal/v0.1.1
//   - bad: wongidle/foobar |
func (gf *GitlabFetcher) Query(ctx context.Context, path, query string) (string, time.Time, error) {
	if err := module.Check(path, query); err != nil {
		slog.Warn("bad path-query pair", slog.String("path", path), slog.String("query", query), slog.String("error", err.Error()))
		return "", time.Time{}, err
	}
	slog.Info("fetch tag from remote host", slog.String("path", path), slog.String("query", query))
	info, err := gf.gitlab.GetTag(ctx, path, query)
	if err != nil {
		slog.Warn("failed to get tag info from gitlab host", slog.String("project", path), slog.String("ref", query), sloghelper.Error(err))
		return "", time.Time{}, err
	}
	return info.Version, info.Time, nil
}

func (fg *GitlabFetcher) List(ctx context.Context, path string) ([]string, error) {
	slog.Info("calling List", slog.String("path", path))
	return nil, nil
}

func (fg *GitlabFetcher) Download(ctx context.Context, path, version string) (info, mod, zip io.ReadSeekCloser, err error) {
	if err := module.Check(path, version); err != nil {
		slog.Warn("bad path-version pair", slog.String("path", path), slog.String("version", version), slog.String("error", err.Error()))
		return nil, nil, nil, err
	}
	return nil, nil, nil, nil
}

func (fg *GitlabFetcher) Extract(ctx context.Context, path, query string) (*Locator, error) {
	if err := module.Check(path, query); err != nil {
		return nil, err
	}
	ps := strings.Split(path, "/") // ["gitlab.com", "wongidle", "mutiples", "pkg", "srv", "v2"]
	// 最简单的模式， host/group/proj  v0/1 版本，大多数情况
	loc := &Locator{Ref: query}

	tail := len(ps) - 1
	for cursor := 2; cursor < len(ps); cursor++ {
		proj := strings.Join(ps[1:cursor+1], "/")
		ok, err := fg.gitlab.IsProject(ctx, proj)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}

		// ["gitlab.com", "wongidle", "foobar", "pkg"]
		loc.Repository = proj
		if cursor < tail {
			// 此时必然包含子路径或v2以上版本，也可能同时满足
			virtualVersion := ps[tail]
			isV2 := matcher.MatchString(virtualVersion)
			if isV2 {
				tail--
			}
			// github.com/foo/bar/xxxx/yyy v1.0.0   invalid version
			// github.com/foo/bar/v2 v2.0.0  foo/bar,  "", v2.0.0
			// github.com/foo/bar/echo/world v1.0.0  echo/world/v1.0.0, world/v1.0.0
			if tail > cursor {
				dirs := ps[cursor+1 : tail+1]
				// 从尾部开始递归
				for index := len(dirs); index > 0; index-- {
					subPath := strings.Join(dirs[0:index], "/")
					ref := subPath + "/" + query
					_, err = fg.gitlab.GetFile(ctx, loc.Repository, subPath+"/go.mod", ref)
					if err != nil {
						slog.Warn("no go.mod founded in subpath", slog.String("project", loc.Repository),
							slog.String("subpath", subPath), slog.String("version", ref), slog.String("error", err.Error()))
						continue
					}
					loc.SubPath = subPath
					loc.Ref = ref
					return loc, nil
				}
				// 运行到这里，只能是错误的模块路径了
				return nil, errors.New("invalid module path")
			}
		}
		return loc, nil
	}
	return nil, fmt.Errorf("cannot found gitlab project with path=%s  query=%s", path, query)
}
