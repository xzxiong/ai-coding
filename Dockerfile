FROM public.ecr.aws/docker/library/golang:1.25 AS builder

ARG GOPROXY="https://goproxy.cn,direct"
ARG GOMODCACHE="/go/pkg/mod"

WORKDIR /go/src/github.com/xzxiong/ai-coding

COPY go.mod go.sum ./
RUN go env -w GOPROXY=${GOPROXY} GOMODCACHE="$GOMODCACHE"
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

FROM public.ecr.aws/ubuntu/ubuntu:22.04

RUN apt-get -qq update \
    && apt-get -qq install -y --no-install-recommends ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /server /server

WORKDIR /
EXPOSE 8080
ENTRYPOINT ["/server"]
