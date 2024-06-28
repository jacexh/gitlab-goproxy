package main

import (
	"net/http"

	"github.com/go-jimu/components/sloghelper"
	"github.com/goproxy/goproxy"
	gp "github.com/jacexh/gitlab-goproxy"
)

func main() {
	_ = sloghelper.NewLog(sloghelper.Options{Output: "console"})

	http.ListenAndServe("localhost:8080", &goproxy.Goproxy{
		ProxiedSumDBs: []string{
			"sum.golang.org https://goproxy.cn/sumdb/sum.golang.org", // 代理默认的校验和数据库
		},
		Fetcher: gp.NewGitlabFetcher(gp.GitlabFetcherConfig{BaseURL: "https://gitlab.com/api/v4"}),
	})
}
