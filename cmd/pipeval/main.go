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
	"strings"
	"time"

	_ "time/tzdata"

	"dgsbot/internal/advisor"
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
	var adv advisor.Advisor
	switch cfg.PlannerMode {
	case config.PlannerStub:
		pl = planner.NewStub()
		nar = narrator.NewTemplate()
		adv = advisor.NewTemplate()
	default:
		cli := llm.New(cfg.LLM)
		pl = planner.NewLLM(cli, cfg.LLM.Model, cfg.LLM.ForceJSON)
		nar = narrator.NewLLM(cli, cfg.LLM.Model)
		adv = advisor.NewLLM(cli, cfg.LLM.Model)
	}

	tenants, err := tenantctx.Load(filepath.Join(cfg.FixturesPath, "tenants.example.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "tenants: %v\n", err)
		os.Exit(1)
	}
	a := app.New(pl, tenants, dooglys.NewFixtureClient(cfg.FixturesPath), resolver.Load(cfg.FixturesPath), nar, adv, session.NewStore())
	// Фиксированное «сейчас» — под даты фикстур (как в интеграционных тестах).
	a.Now = func() time.Time { return time.Date(2026, 6, 19, 10, 0, 0, 0, time.UTC) }

	fmt.Printf("pipeval: %d кейсов, planner=%s, данные=фикстуры(%s)\n\n", len(cases), cfg.PlannerMode, cfg.FixturesPath)

	dump := os.Getenv("PIPEVAL_DUMP") != "" // показать план + текст ответа (качественная оценка)

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
		if dump {
			pl := r.Answer.Plan
			fmt.Printf("        план: intent=%s report=%s method=%s order=%s period=%s metrics=%v\n",
				pl.EffectiveIntent(), pl.Report, pl.Method, pl.Order, pl.Period.Token, pl.Metrics)
			fmt.Printf("        ─── ответ ───\n")
			for _, line := range strings.Split(strings.TrimRight(r.Answer.Text, "\n"), "\n") {
				fmt.Printf("        | %s\n", line)
			}
			fmt.Println()
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
