FROM public.ecr.aws/docker/library/golang:1.25 AS builder

WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

FROM public.ecr.aws/ubuntu/ubuntu:22.04

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /server /server

EXPOSE 8080
ENTRYPOINT ["/server"]
