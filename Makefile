.PHONY: build run clean test docker-build docker-up docker-down e2e

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

clean:
	rm -rf bin/

test:
	go test ./...

docker-build:
	docker compose build

docker-up:
	docker compose up -d

docker-down:
	docker compose down

e2e:
	pip install -q -r tests/e2e/requirements.txt
	PROXY_URL=http://localhost:8080 pytest tests/e2e/test_anthropic_sdk.py -v
