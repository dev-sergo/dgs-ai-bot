// Command bot — Telegram-точка входа Dooglys AI-bot. Отдельный процесс/деплой
// от HTTP-сервера (cmd/server), общая сборка app.App — internal/bootstrap.
package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"

	_ "time/tzdata" // встроенная база таймзон — чтобы работало в distroless без системного tzdata

	"dgsbot/internal/bootstrap"
	"dgsbot/internal/config"
	tgx "dgsbot/internal/transport/telegram"
)

func main() {
	cfg := config.Load()
	log.Printf("config: %s", cfg.Summary())

	if cfg.Telegram.Token == "" {
		log.Fatal("TELEGRAM_TOKEN не задан — боту нечем подключиться к Telegram")
	}

	a, cleanup, err := bootstrap.App(cfg)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer cleanup()

	bot, err := tgx.New(cfg.Telegram, a)
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	bot.Run(ctx) // блокирует до отмены ctx (SIGINT/SIGTERM)
}
