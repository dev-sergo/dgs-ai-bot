# Go живёт только в контейнере — на хосте Go не нужен.
GO_IMAGE ?= golang:1.23
PWD_DIR  := $(shell pwd)

# Кэши модулей и сборки переживают перезапуски (именованные volume).
GO_RUN = docker run --rm \
	-v $(PWD_DIR):/app -w /app \
	-v dgsbot-gomod:/go/pkg/mod \
	-v dgsbot-gocache:/root/.cache/go-build \
	-e GOFLAGS=-buildvcs=false \
	-e CGO_ENABLED=0 \
	$(GO_IMAGE)

.PHONY: tidy build test vet run bench fmt sh

tidy:        ## go mod tidy
	$(GO_RUN) go mod tidy

build:       ## собрать бинарник в ./bin
	$(GO_RUN) go build -o bin/server ./cmd/server

test:        ## юнит/интеграционные тесты (без GPU)
	$(GO_RUN) go test ./...

vet:         ## статанализ
	$(GO_RUN) go vet ./...

fmt:         ## форматирование
	$(GO_RUN) gofmt -l -w .

run:         ## поднять сервис через docker-compose
	docker compose up --build

bench:       ## eval-бенчмарк против реальной модели (нужен доступ к LLM-ригу)
	$(GO_RUN) go test ./test/eval/... -run TestEval -v

sh:          ## shell в go-контейнере
	$(GO_RUN) sh
