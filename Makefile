IMAGE_NAME ?= ai-coding
IMAGE_TAG  ?= latest

.PHONY: build run clean test test-real image docker-build docker-up docker-down e2e help

build:
	go build -o bin/server ./cmd/server

run:
	go run ./cmd/server

clean:
	rm -rf bin/

test:
	go test ./...

test-real:
	@test -n "$$TEST_BASE_URL" || (echo "ERROR: TEST_BASE_URL is not set"; exit 1)
	@test -n "$$TEST_TOKEN" || (echo "ERROR: TEST_TOKEN is not set"; exit 1)
	go test -v -count=1 -run TestReal ./tests/e2e/

image:
	docker build -t $(IMAGE_NAME):$(IMAGE_TAG) .

docker-build: image

docker-up:
	docker compose up -d

docker-down:
	docker compose down

e2e:
	pip install -q -r tests/e2e/requirements.txt
	PROXY_URL=http://localhost:8080 pytest tests/e2e/test_anthropic_sdk.py -v

help:
	@echo "Usage: make <target>"
	@echo ""
	@echo "Targets:"
	@echo "  build         Build server binary to bin/server"
	@echo "  run           Run server locally"
	@echo "  test          Run unit tests"
	@echo "  test-real     Run real API e2e tests (requires TEST_BASE_URL, TEST_TOKEN)"
	@echo "  e2e           Run Python Anthropic SDK e2e tests"
	@echo "  image         Build Docker image (IMAGE_NAME, IMAGE_TAG)"
	@echo "  docker-build  Build Docker image via compose"
	@echo "  docker-up     Start containers"
	@echo "  docker-down   Stop containers"
	@echo "  clean         Remove build artifacts"
	@echo "  help          Show this help"
