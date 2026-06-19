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

# Хостовая ОС/арх — чтобы запускать нативный бинарь на маке (минуя сеть Docker-VM).
HOST_OS   := $(shell uname -s | tr '[:upper:]' '[:lower:]')
HOST_UARCH := $(shell uname -m)
HOST_ARCH := $(if $(filter x86_64,$(HOST_UARCH)),amd64,$(if $(filter arm64 aarch64,$(HOST_UARCH)),arm64,$(HOST_UARCH)))

.PHONY: tidy build build-host run-host test vet run eval-host fmt sh

tidy:        ## go mod tidy
	$(GO_RUN) go mod tidy

build:       ## собрать бинарник (linux, для контейнера) в ./bin
	$(GO_RUN) go build -o bin/server ./cmd/server

build-host:  ## кросс-собрать нативный бинарь под текущий мак (./bin/server-host)
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/server-host ./cmd/server"

run-host: build-host  ## запустить сервис НА ХОСТЕ (использует сеть мака → видит риг)
	PLANNER_MODE=$${PLANNER_MODE:-llm} \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	FIXTURES_PATH=docs/contracts/fixtures \
	./bin/server-host

test:        ## юнит/интеграционные тесты (без GPU)
	$(GO_RUN) go test ./...

vet:         ## статанализ
	$(GO_RUN) go vet ./...

fmt:         ## форматирование
	$(GO_RUN) gofmt -l -w .

run:         ## поднять сервис через docker-compose
	docker compose up --build

eval-host:   ## eval-бенчмарк планировщика против рига (НА ХОСТЕ — нужен доступ к LLM)
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/eval-host ./cmd/eval"
	PLANNER_MODE=llm \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	EVAL_PROMPTS=$${EVAL_PROMPTS:-test/eval/prompts.jsonl} \
	./bin/eval-host

sh:          ## shell в go-контейнере
	$(GO_RUN) sh
