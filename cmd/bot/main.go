// Command bot — Telegram-точка входа Dooglys AI-bot. Отдельный процесс/деплой
// от HTTP-сервера (cmd/server), общая сборка app.App — internal/bootstrap.
package main

import (
	"context"
	"log"
	"os/signal"
	"sync"
	"syscall"

	_ "time/tzdata" // встроенная база таймзон — чтобы работало в distroless без системного tzdata

	"dgsbot/internal/bootstrap"
	"dgsbot/internal/config"
	tgx "dgsbot/internal/transport/telegram"
)

func main() {
	cfg := config.Load()
	log.Printf("config: %s", cfg.Summary())

	// Каждый тенант обязан нести токен бота — иначе явный fail на старте (не 500 позже).
	if err := cfg.ValidateTelegram(); err != nil {
		log.Fatalf("telegram config: %v", err)
	}

	a, cleanup, err := bootstrap.App(cfg)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}
	defer cleanup()

	// Поднимаем по боту на тенанта (3-бот режим). Каждый жёстко на своём tenantID со
	// своим whitelist'ом; общий app.App резолвит источник данных по tenantID (изоляция).
	bots := make([]*tgx.Bot, 0, len(cfg.Tenants))
	for _, tc := range cfg.Tenants {
		bot, err := tgx.NewFromTenant(tc, a)
		if err != nil {
			log.Fatalf("telegram[%s]: %v", tc.ID, err)
		}
		bots = append(bots, bot)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var wg sync.WaitGroup
	for _, bot := range bots {
		wg.Add(1)
		go func(b *tgx.Bot) {
			defer wg.Done()
			b.Run(ctx) // блокирует до отмены ctx (SIGINT/SIGTERM)
		}(bot)
	}
	wg.Wait()
}
