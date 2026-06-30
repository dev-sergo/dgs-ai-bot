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

.PHONY: tidy get-tg build build-host run-host bot-host test test-live vet run eval-host fmt sh

tidy:        ## go mod tidy
	$(GO_RUN) go mod tidy

get-tg:      ## добавить telegram-bot-api в go.mod (нужно один раз)
	$(GO_RUN) sh -c "go get github.com/go-telegram-bot-api/telegram-bot-api/v5@latest && go mod tidy"

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

bot-host:    ## поднять Telegram-бот НА ХОСТЕ (нужен TELEGRAM_TOKEN в окружении)
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/bot ./cmd/bot"
	PLANNER_MODE=$${PLANNER_MODE:-llm} \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	FIXTURES_PATH=$${FIXTURES_PATH:-docs/contracts/fixtures} \
	TELEGRAM_TENANT=$${TELEGRAM_TENANT:-mock_single} \
	./bin/bot

test:        ## юнит/интеграционные тесты (без GPU)
	$(GO_RUN) go test ./...

test-live:   ## боевая проверка Report-API (нужны DGS_ACCESS_TOKEN, DGS_DOMAIN; опц. DGS_REPORT_BASE)
	docker run --rm -v $(PWD_DIR):/app -w /app \
		-v dgsbot-gomod:/go/pkg/mod -v dgsbot-gocache:/root/.cache/go-build \
		-e GOFLAGS=-buildvcs=false -e CGO_ENABLED=0 \
		-e DGS_REPORT_BASE='$(or $(DGS_REPORT_BASE),https://api.dooglys.com/api/v1/reports)' \
		-e DGS_ACCESS_TOKEN='$(DGS_ACCESS_TOKEN)' \
		-e DGS_DOMAIN='$(or $(DGS_DOMAIN),rukagreka)' \
		$(GO_IMAGE) \
		go test -tags live ./internal/dooglys/ -run TestLiveReport -v

vet:         ## статанализ
	$(GO_RUN) go vet ./...

fmt:         ## форматирование
	$(GO_RUN) gofmt -l -w .

run:         ## поднять сервис через docker-compose
	docker compose up --build

eval-host:   ## eval-бенчмарк планировщика против рига (НА ХОСТЕ — нужен доступ к LLM)
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/eval-host ./cmd/eval"
	@corpus=$$(basename $${EVAL_PROMPTS:-test/eval/prompts.jsonl} .jsonl); \
	out="bench/runs/$$(date +%Y-%m-%d_%H%M)_eval_$${corpus}_llm_$$(git rev-parse --short HEAD).log"; \
	PLANNER_MODE=llm \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	EVAL_PROMPTS=$${EVAL_PROMPTS:-test/eval/prompts.jsonl} \
	./bin/eval-host 2>&1 | tee "$$out"; \
	bash bench/summarize.sh "$$out" > "$${out%.log}.json"; \
	echo "bench: $$out"

serve-host:  ## поднять сервер НА ХОСТЕ с реальной LLM (достаёт LAN-риг, как eval-host) → http://localhost:8088
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/server ./cmd/server"
	PLANNER_MODE=llm \
	HTTP_ADDR=$${HTTP_ADDR:-:8088} \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	FIXTURES_PATH=$${FIXTURES_PATH:-docs/contracts/fixtures} \
	./bin/server

pipeval:     ## full-pipeline бенчмарк по ответу (Stub+фикстуры, детерминированно, без рига)
	$(GO_RUN) sh -c "PLANNER_MODE=stub go run ./cmd/pipeval"

pipeval-host: ## full-pipeline бенчмарк через реальный LLM (НА ХОСТЕ — нужен риг)
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/pipeval-host ./cmd/pipeval"
	@corpus=$$(basename $${PIPEVAL_CASES:-test/eval/pipeline.jsonl} .jsonl); \
	out="bench/runs/$$(date +%Y-%m-%d_%H%M)_pipeval_$${corpus}_llm_$$(git rev-parse --short HEAD).log"; \
	PLANNER_MODE=llm \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	PIPEVAL_CASES=$${PIPEVAL_CASES:-test/eval/pipeline.jsonl} \
	./bin/pipeval-host 2>&1 | tee "$$out"; \
	bash bench/summarize.sh "$$out" > "$${out%.log}.json"; \
	echo "bench: $$out"

pipeval-followups-host: ## follow-up (контекст диалога) через реальный LLM — host-only (Stub историю игнорирует)
	$(MAKE) pipeval-host PIPEVAL_CASES=test/eval/pipeline-followups.jsonl

pipeval-quality-host: ## КАЧЕСТВЕННЫЙ прогон: печатает план + ТЕКСТ ОТВЕТА по каждому запросу (реальная LLM)
	$(GO_RUN) sh -c "GOOS=$(HOST_OS) GOARCH=$(HOST_ARCH) go build -o bin/pipeval-host ./cmd/pipeval"
	@out="bench/runs/$$(date +%Y-%m-%d_%H%M)_pipeval_quality_llm_$$(git rev-parse --short HEAD).log"; \
	PLANNER_MODE=llm PIPEVAL_DUMP=1 \
	LLM_BASE_URL=$${LLM_BASE_URL:-http://172.20.10.2:8080} \
	LLM_MODEL=$${LLM_MODEL:-qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07} \
	PIPEVAL_CASES=test/eval/quality.jsonl \
	./bin/pipeval-host 2>&1 | tee "$$out"; \
	bash bench/summarize.sh "$$out" > "$${out%.log}.json"; \
	echo "bench: $$out"

sh:          ## shell в go-контейнере
	$(GO_RUN) sh
