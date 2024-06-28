package gitlabgoproxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jacexh/requests"
	"github.com/xanzy/go-gitlab"
)

type GitlabHost struct {
	conf    GitlabFetcherConfig
	client  *gitlab.Client
	session *requests.Session
}

var _ GitLab = (*GitlabHost)(nil)

func NewGitlabHost(conf GitlabFetcherConfig) *GitlabHost {
	client, err := gitlab.NewClient("", gitlab.WithBaseURL(conf.BaseURL))
	if err != nil {
		panic(err)
	}
	session := requests.NewSession()
	gh := &GitlabHost{client: client, session: session, conf: conf}
	if gh.conf.AccessToken != "" {
		session.Apply(requests.WithGlobalHeader(requests.Any{"PRIVATE-TOKEN": gh.conf.AccessToken}))
	}
	return gh
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
	o := "version"
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
	endpoint := fmt.Sprintf("%s/projects/%s/repository/files/%s/raw", gh.conf.BaseURL, url.PathEscape(repo), url.PathEscape(path))
	res, data, err := gh.session.GetWithContext(ctx, endpoint, requests.Params{Query: requests.Any{"ref": ref}}, nil)
	if err != nil {
		return nil, err
	}
	return data, gitlab.CheckResponse(res)
}

func (gh *GitlabHost) Download(ctx context.Context, repo, dir, ref string) (io.Reader, error) {
	format := "zip"
	data, _, err := gh.client.Repositories.Archive(
		repo,
		&gitlab.ArchiveOptions{Format: &format, Path: &dir, SHA: &ref},
		gitlab.WithContext(ctx),
	)
	if err != nil {
		return nil, err
	}
	buf := bytes.NewReader(data)
	return buf, nil
}
