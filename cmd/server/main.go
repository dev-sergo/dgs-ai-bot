// Command server — точка входа Dooglys AI-bot.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"dgsbot/internal/config"
	"dgsbot/internal/llm"
	"dgsbot/internal/planner"
	httpx "dgsbot/internal/transport/http"
)

func main() {
	cfg := config.Load()
	log.Printf("config: %s", cfg.Summary())

	var pl planner.Planner
	switch cfg.PlannerMode {
	case config.PlannerStub:
		pl = planner.NewStub()
	default:
		pl = planner.NewLLM(llm.New(cfg.LLM), cfg.LLM.Model)
	}

	srv := httpx.New(cfg, pl)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Run(ctx); err != nil {
		log.Printf("server stopped: %v", err)
		os.Exit(1)
	}
}
