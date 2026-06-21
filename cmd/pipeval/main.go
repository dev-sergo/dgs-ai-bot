// Command pipeval — full-pipeline бенчмарк: прогон через app.Ask со сверкой
// ИТОГОВОГО ОТВЕТА (числа/текст/нарратив/утечки), а не плана.
//
// По умолчанию PLANNER_MODE=stub — детерминированно, без рига (для CI и быстрой
// проверки движка/рендера). PLANNER_MODE=llm гонит реальный LLM по всему пути
// (см. `make pipeval-host`). Данные всегда из фикстур, поэтому числа предсказуемы.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "time/tzdata"

	"dgsbot/internal/app"
	"dgsbot/internal/config"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/llm"
	"dgsbot/internal/narrator"
	"dgsbot/internal/pipeval"
	"dgsbot/internal/planner"
	"dgsbot/internal/resolver"
	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
)

func main() {
	cfg := config.Load()
	path := envOr("PIPEVAL_CASES", "test/eval/pipeline.jsonl")
	tenantID := envOr("PIPEVAL_TENANT", "mock_single")

	cases, err := pipeval.LoadCases(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка загрузки %s: %v\n", path, err)
		os.Exit(1)
	}

	var pl planner.Planner
	var nar narrator.Narrator
	switch cfg.PlannerMode {
	case config.PlannerStub:
		pl = planner.NewStub()
		nar = narrator.NewTemplate()
	default:
		cli := llm.New(cfg.LLM)
		pl = planner.NewLLM(cli, cfg.LLM.Model, cfg.LLM.ForceJSON)
		nar = narrator.NewLLM(cli, cfg.LLM.Model)
	}

	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tenants: %v\n", err)
		os.Exit(1)
	}
	a := app.New(pl, tenants, dooglys.NewFixtureClient(cfg.FixturesPath), resolver.Load(cfg.FixturesPath), nar, session.NewStore())
	// Фиксированное «сейчас» — под даты фикстур (как в интеграционных тестах).
	a.Now = func() time.Time { return time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC) }

	fmt.Printf("pipeval: %d кейсов, planner=%s, данные=фикстуры(%s)\n\n", len(cases), cfg.PlannerMode, cfg.FixturesPath)

	results := pipeval.Run(context.Background(), a, tenantID, cases)
	for _, r := range results {
		status := "PASS"
		switch {
		case r.Err != nil:
			status = "ERR "
		case !r.Pass():
			status = "FAIL"
		}
		fmt.Printf("[%s] %5dms  %s\n", status, r.LatencyMS, r.Query)
		if r.Err != nil {
			fmt.Printf("        ошибка: %v\n", r.Err)
			continue
		}
		for _, m := range r.Mismatch {
			fmt.Printf("        ✗ %s\n", m)
		}
	}

	s := pipeval.Summarize(results)
	fmt.Printf("\n— итог —\nпрошло: %d/%d\nошибок: %d\nлатентность: p50=%dms p95=%dms max=%dms\n",
		s.Passed, s.Total, s.Errors, s.LatP50, s.LatP95, s.LatMax)

	if s.Passed < s.Total {
		os.Exit(2)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
