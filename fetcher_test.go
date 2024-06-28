package gitlabgoproxy_test

import (
	"context"
	"io"
	"log"
	"log/slog"
	"strings"
	"testing"
	"time"

	gitlabgoproxy "github.com/jacexh/gitlab-goproxy"
	"github.com/stretchr/testify/assert"
)

func TestGitlabFetcher_Extract(t *testing.T) {
	fetcher := gitlabgoproxy.NewGitlabFetcher(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"}).(*gitlabgoproxy.GitlabFetcher)
	// simple
	loc, err := fetcher.Extract(context.Background(), "gitlab.com/wongidle/foobar", "v0.2.0")
	assert.NoError(t, err)
	assert.EqualValues(t, &gitlabgoproxy.Locator{Repository: "wongidle/foobar", SubPath: "", Ref: "v0.2.0"}, loc)

	// invalid module path
	_, err = fetcher.Extract(context.Background(), "gitlab.com/wongidle/foobar/internal/pkg", "v0.2.0")
	assert.Error(t, err)

	// submodule path
	loc, err = fetcher.Extract(context.Background(), "gitlab.com/wongidle/foobar/pkg", "v0.2.1")
	assert.NoError(t, err)
	assert.EqualValues(t, &gitlabgoproxy.Locator{Repository: "wongidle/foobar", SubPath: "pkg", Ref: "pkg/v0.2.1"}, loc)

	// v2
	loc, err = fetcher.Extract(context.Background(), "gitlab.com/wongidle/mutiples/v2", "v2.0.2")
	assert.NoError(t, err)
	assert.EqualValues(t, &gitlabgoproxy.Locator{Repository: "wongidle/mutiples", SubPath: "", Ref: "v2.0.2"}, loc)

	// v2 submodule
	loc, err = fetcher.Extract(context.Background(), "gitlab.com/wongidle/mutiples/pkg/str/v2", "v2.0.2")
	assert.NoError(t, err)
	assert.EqualValues(t, &gitlabgoproxy.Locator{Repository: "wongidle/mutiples", SubPath: "pkg/str", Ref: "pkg/str/v2.0.2"}, loc)

	// v2 invalid module
	_, err = fetcher.Extract(context.Background(), "gitlab.com/wongidle/mutiples/internal/pkg/bytesconv/v2", "v2.0.2")
	assert.Error(t, err)
	_, err = fetcher.Extract(context.Background(), "gitlab.com/wongidle/mutiples/v2/internal/pkg/bytesconv", "v2.0.2")
	assert.Error(t, err)
}

func TestGitlabFetcher_List(t *testing.T) {
	fetcher := gitlabgoproxy.NewGitlabFetcher(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"})

	// simple
	versions, err := fetcher.List(context.Background(), "gitlab.com/wongidle/foobar")
	assert.NoError(t, err)
	assert.EqualValues(t, versions, []string{"v0.1.0", "v0.1.1", "v0.2.0"})

	// v1 sub module
	versions, err = fetcher.List(context.Background(), "gitlab.com/wongidle/foobar/pkg")
	assert.NoError(t, err)
	assert.EqualValues(t, versions, []string{"v0.2.0", "v0.2.1"})

	// bad module
	_, err = fetcher.List(context.Background(), "gitlab.com/wongidle/foobar/internal/pkg")
	assert.Error(t, err)

	// v2
	versions, err = fetcher.List(context.Background(), "gitlab.com/wongidle/mutiples/v2")
	assert.NoError(t, err)
	assert.EqualValues(t, versions, []string{"v2.0.1", "v2.0.2"})

	// v2 submodule
	versions, err = fetcher.List(context.Background(), "gitlab.com/wongidle/mutiples/pkg/str/v2")
	assert.NoError(t, err)
	assert.EqualValues(t, versions, []string{"v2.0.2"})

	// v2 invalid
	_, err = fetcher.List(context.Background(), "gitlab.com/wongidle/mutiples/internal/pkg/bytesconv/v2")
	assert.Error(t, err)

	versions, err = fetcher.List(context.Background(), "gitlab.com/wongidle/mutiples/v2/internal/pkg/bytesconv")
	slog.Info("unexpected versions", slog.Any("versions", versions))
	assert.Error(t, err)
}

func TestGitlabFetcher_Download(t *testing.T) {
	fetcher := gitlabgoproxy.NewGitlabFetcher(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"}).(*gitlabgoproxy.GitlabFetcher)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, mod, _, err := fetcher.Download(ctx, "gitlab.com/wongidle/foobar", "v0.2.0")
	assert.NoError(t, err)

	data, err := io.ReadAll(mod)
	assert.NoError(t, err)
	slog.Info(string(data), slog.Int("lenght", len(data)))

	// time.Sleep(30 * time.Second)
}

func TestGitlabFetcher_GoMode(t *testing.T) {
	fetcher := gitlabgoproxy.NewGitlabFetcher(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"}).(*gitlabgoproxy.GitlabFetcher)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	reader, err := fetcher.SaveGoMod(ctx, &gitlabgoproxy.Locator{Repository: "wongidle/foobar", Ref: "v0.2.0"})
	assert.NoError(t, err)
	data, err := io.ReadAll(reader)
	assert.NoError(t, err)
	log.Println(string(data))
}

func TestSplit(t *testing.T) {
	p := ""
	ps := strings.Split(p, "/")
	log.Println(len(ps))
}
