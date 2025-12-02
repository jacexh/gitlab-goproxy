package gitlabgoproxy

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/xanzy/go-gitlab"
)

type GitlabHost struct {
	conf   GitlabFetcherConfig
	client *gitlab.Client
}

var _ GitLab = (*GitlabHost)(nil)

func NewGitlabHost(conf GitlabFetcherConfig) (*GitlabHost, error) {
	client, err := gitlab.NewClient(conf.AccessToken, gitlab.WithBaseURL(conf.Endpoint))
	if err != nil {
		return nil, err
	}
	gh := &GitlabHost{client: client, conf: conf}
	return gh, nil
}

func (gh *GitlabHost) IsProject(ctx context.Context, repo string) (bool, error) {
	_, _, err := gh.client.Projects.GetProject(repo, &gitlab.GetProjectOptions{}, gitlab.WithContext(ctx))
	if err == nil {
		return true, nil
	}
	er, ok := err.(*gitlab.ErrorResponse)
	if ok && er.Response.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

func (gh *GitlabHost) ListTags(ctx context.Context, repo string, prefix string) ([]*Info, error) {
	o := "name"
	s := "asc"
	pre := new(string)
	if prefix != "" {
		*pre = "^" + prefix
	}
	opt := &gitlab.ListTagsOptions{
		ListOptions: gitlab.ListOptions{Page: 1, PerPage: 100},
		OrderBy:     &o,
		Sort:        &s,
		Search:      pre,
	}

	ret := make([]*Info, 0)

	for {
		tags, _, err := gh.client.Tags.ListTags(repo, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, err
		}
		for _, tag := range tags {
			ret = append(ret, &Info{Version: tag.Name, Time: *tag.Commit.CreatedAt})
		}
		if len(tags) < 100 {
			return ret, nil
		}
		opt.ListOptions.Page += 1
	}
}

func (gh *GitlabHost) GetTag(ctx context.Context, repo, tag string) (*Info, error) {
	t, _, err := gh.client.Tags.GetTag(repo, tag, gitlab.WithContext(ctx))
	if err != nil {
		return nil, err
	}
	return &Info{Version: t.Name, Time: *t.Commit.CreatedAt}, nil
}

func (gh *GitlabHost) GetFile(ctx context.Context, repo, path, ref string) ([]byte, error) {
	opt := &gitlab.GetRawFileOptions{Ref: &ref}
	data, _, err := gh.client.RepositoryFiles.GetRawFile(repo, path, opt, gitlab.WithContext(ctx))
	return data, err
}

func (gh *GitlabHost) Download(ctx context.Context, repo, dir, ref string) (io.Reader, error) {
	format := "zip"
	opt := &gitlab.ArchiveOptions{Format: &format, SHA: &ref}
	if dir != "" {
		opt.Path = &dir
	}
	data, _, err := gh.client.Repositories.Archive(
		repo,
		opt,
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewReader(data)
	return buf, nil
}
