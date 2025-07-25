FROM golang:1.24-bookworm AS builder
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

EXPOSE 8080
CMD ["/app/gitlab-goproxy"]