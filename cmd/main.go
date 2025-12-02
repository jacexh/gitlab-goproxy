package main

import (
	"log/slog"
	"net/http"

	"github.com/go-jimu/components/config/loader"
	"github.com/go-jimu/components/sloghelper"
	"github.com/goproxy/goproxy"
	gp "github.com/jacexh/gitlab-goproxy"
)

func parseConfig() (gp.Config, error) {
	conf := new(gp.Config)
	if err := loader.Load(conf); err != nil {
		return gp.Config{}, err
	}
	return *conf, nil
}

func main() {
	_ = sloghelper.NewLog(sloghelper.Options{Output: "console"})
	conf, err := parseConfig()
	if err != nil {
		slog.Error("failed to load configs", sloghelper.Error(err))
		return
	}

	slog.Info("loaded configs", slog.Any("config", conf))

	cacher, err := gp.NewS3Cache(conf.S3)
	if err != nil {
		slog.Warn("failed to enable cacher", slog.String("error", err.Error()))
	}

	fetcher, err := gp.NewMixedFetcher(conf)
	if err != nil {
		slog.Error("failed to initialize mixed fetcher", sloghelper.Error(err))
		return
	}

	http.ListenAndServe(":8080", &goproxy.Goproxy{
		// ProxiedSumDBs: []string{
		// 	"sum.golang.org https://goproxy.cn/sumdb/sum.golang.org", // Proxy default checksum database
		// },
		Fetcher: fetcher,
		Cacher:  cacher,
	})
}
