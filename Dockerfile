# --- build ---
FROM golang:1.23 AS build
WORKDIR /app
ENV CGO_ENABLED=0 GOFLAGS=-buildvcs=false
COPY go.mod ./
# (зависимостей пока нет; когда появятся — добавить go.sum и `go mod download`)
COPY . .
RUN go build -o /out/server ./cmd/server

# --- runtime ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/server /app/server
# Фикстуры нужны FixtureClient'у на M1+.
COPY docs/contracts/fixtures /app/docs/contracts/fixtures
EXPOSE 8088
ENTRYPOINT ["/app/server"]
