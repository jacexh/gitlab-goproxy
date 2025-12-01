FROM golang:1.25-bookworm AS builder
WORKDIR /go/src
COPY . /go/src/
RUN set -e \
    && export GOPROXY=https://goproxy.cn,direct \
    && go mod download \
    && go build -ldflags "-w -s -extldflags '-static'" -tags netgo -o gitlab-goproxy cmd/main.go \
    && apt update -yqq \
    && apt install -yqq ca-certificates
# https://valyala.medium.com/stripping-dependency-bloat-in-victoriametrics-docker-image-983fb5912b0d

FROM debian:bookworm
WORKDIR /app
COPY --from=builder /go/src/gitlab-goproxy .
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY ./configs /app/configs
RUN set -e \
    && apt update -yqq \
    && apt install -y --no-install-recommends git git-lfs gpg subversion fossil mercurial \
    && git lfs install --system \
    && rm -rf /var/lib/apt/lists/* 

EXPOSE 8080
CMD ["/app/gitlab-goproxy"]