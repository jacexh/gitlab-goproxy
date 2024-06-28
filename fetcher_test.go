package gitlabgoproxy_test

import (
	"context"
	"testing"

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
