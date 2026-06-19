// Command eval — прогон eval-набора через реальную модель (запускать на хосте: see `make eval-host`).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"dgsbot/internal/catalog"
	"dgsbot/internal/config"
	"dgsbot/internal/eval"
	"dgsbot/internal/llm"
	"dgsbot/internal/planner"
)

func main() {
	cfg := config.Load()
	path := envOr("EVAL_PROMPTS", "test/eval/prompts.jsonl")

	cases, err := loadCases(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ошибка загрузки %s: %v\n", path, err)
		os.Exit(1)
	}

	cli := llm.New(cfg.LLM)
	pl := planner.NewLLM(cli, cfg.LLM.Model, cfg.LLM.ForceJSON)

	fmt.Printf("eval: %d кейсов, модель=%s, эндпоинт=%s\n\n", len(cases), cfg.LLM.Model, cfg.LLM.BaseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	results := eval.Run(ctx, pl, catalog.Default(), cases)

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
		fmt.Printf("        план: report=%s class=%s method=%s period=%s\n",
			r.Plan.Report, r.Plan.Class, r.Plan.Method, r.Plan.Period.Token)
		for _, m := range r.Mismatch {
			fmt.Printf("        ✗ %s\n", m)
		}
	}

	s := eval.Summarize(results)
	fmt.Printf("\n— итог —\n")
	fmt.Printf("прошло:   %d/%d\n", s.Passed, s.Total)
	fmt.Printf("валидных: %d/%d\n", s.Valid, s.Total)
	fmt.Printf("ошибок:   %d\n", s.Errors)
	fmt.Printf("латентность: p50=%dms p95=%dms max=%dms\n", s.LatP50, s.LatP95, s.LatMax)

	if s.Passed < s.Total {
		os.Exit(2) // ненулевой код — удобно для CI/повторов
	}
}

func loadCases(path string) ([]eval.Case, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cases []eval.Case
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var c eval.Case
		if err := json.Unmarshal(line, &c); err != nil {
			return nil, fmt.Errorf("строка %q: %w", string(line), err)
		}
		cases = append(cases, c)
	}
	return cases, sc.Err()
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
