package gitlabgoproxy_test

import (
	"context"
	"io"
	"log"
	"os"
	"testing"
	"time"

	gitlabgoproxy "github.com/jacexh/gitlab-goproxy"
	"github.com/stretchr/testify/assert"
)

func TestGitHost(t *testing.T) {
	git := gitlabgoproxy.NewGitlabHost(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"})
	tags, err := git.ListTags(context.Background(), "WhyNotHugo/darkman", "v1")
	assert.NoError(t, err)

	for _, tag := range tags {
		log.Println(tag)
	}

	_, err = git.GetTag(context.Background(), "WhyNotHugo/darkman", "v1.5.4")
	assert.NoError(t, err)

	data, err := git.GetFile(context.Background(), "wongidle/mutiples", "pkg/str/go.mod", "pkg/str/v2.0.2")
	assert.NoError(t, err)
	log.Println(string(data))
}

func TestGitHostIsProject(t *testing.T) {
	git := gitlabgoproxy.NewGitlabHost(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"})
	exists, err := git.IsProject(context.Background(), "wongidle/mutiples")
	assert.NoError(t, err)
	log.Println(exists)
}

func TestDownload(t *testing.T) {
	git := gitlabgoproxy.NewGitlabHost(gitlabgoproxy.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader, err := git.Download(ctx, "wongidle/mutiples", "pkg/str", "pkg/str/v2.0.2")
	assert.NoError(t, err)

	file, err := os.Create("temp.zip")
	assert.NoError(t, err)

	defer os.Remove("temp.zip")
	defer file.Close()

	_, err = io.Copy(file, reader)
	assert.NoError(t, err)
}
