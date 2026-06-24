# --- build ---
FROM golang:1.23 AS build
WORKDIR /app
ENV CGO_ENABLED=0 GOFLAGS=-buildvcs=false
COPY go.mod ./
# (зависимостей пока нет; когда появятся — добавить go.sum и `go mod download`)
COPY . .
RUN go build -o /out/server ./cmd/server
# Пустой каталог под датасет вопрос→план→ответ (QUERY_LOG_PATH=/app/data/queries.jsonl).
# Создаём в build-стейдже, чтобы ниже отдать его nonroot'у с правом записи.
RUN mkdir -p /out/data

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
# Фикстуры нужны FixtureClient'у на M1+.
COPY docs/contracts/fixtures /app/docs/contracts/fixtures
# Каталог датасета — владелец nonroot (uid 65532), иначе distroless-nonroot не запишет
# (корень /app принадлежит root). Сюда же монтируется volume из docker-compose.
COPY --from=build --chown=65532:65532 /out/data /app/data
EXPOSE 8088
ENTRYPOINT ["/app/server"]
