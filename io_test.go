package gitlabgoproxy_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	gitlabgoproxy "github.com/jacexh/gitlab-goproxy"
	"github.com/stretchr/testify/assert"
)

func TestAutoClean(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	r := bytes.NewReader([]byte("hello world"))
	reader, _, err := gitlabgoproxy.Save(ctx, r)
	assert.NoError(t, err)
	time.Sleep(5 * time.Second)
	reader.Close()
}
