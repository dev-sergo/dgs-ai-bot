// Command eval — прогон eval-набора через реальную модель (запускать на хосте: see `make eval-host`).
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

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
		fmt.Fprintf(os.Stderr, "load error %s: %v\n", path, err)
		os.Exit(1)
	}

	cli := llm.New(cfg.LLM)
	pl := planner.NewLLM(cli, cfg.LLM.Model, cfg.LLM.ForceJSON)

	// EVAL_CONCURRENCY — сколько запросов к модели держать «в полёте» разом (default 1 =
	// последовательно, чистые замеры latency; >1 использует батчинг vLLM и режет wall-time).
	concurrency := 1
	if v := os.Getenv("EVAL_CONCURRENCY"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			concurrency = n
		}
	}

	total := len(cases)
	fmt.Printf("eval: %d cases, model=%s, endpoint=%s, concurrency=%d\n\n", total, cfg.LLM.Model, cfg.LLM.BaseURL, concurrency)

	// Без глобального потолка на весь прогон: таймаут — на каждый запрос внутри Run
	// (иначе длинный набор упирается в общий дедлайн и хвост падает в context deadline).
	// Печатаем КАЖДЫЙ кейс по мере готовности (callback) — живой прогресс [i/N]. При
	// concurrency>1 строки идут вперемешку (кейсы финишируют не по порядку) — это норм.
	results := eval.Run(context.Background(), pl, catalog.Default(), cases, concurrency, func(i int, r eval.Result) {
		printResult(i+1, total, r)
	})

	s := eval.Summarize(results)
	fmt.Printf("\n— summary —\n")
	fmt.Printf("passed: %d/%d\n", s.Passed, s.Total)
	fmt.Printf("valid:  %d/%d\n", s.Valid, s.Total)
	fmt.Printf("errors: %d\n", s.Errors)
	fmt.Printf("latency: p50=%dms p95=%dms max=%dms\n", s.LatP50, s.LatP95, s.LatMax)

	if s.Passed < s.Total {
		os.Exit(2) // ненулевой код — удобно для CI/повторов
	}
}

// printResult печатает один кейс по мере готовности: префикс [n/total] + статус,
// латентность, запрос, итоговый план и расхождения. Живой прогресс на долгом прогоне.
func printResult(n, total int, r eval.Result) {
	status := "PASS"
	switch {
	case r.Err != nil:
		status = "ERR "
	case !r.Pass():
		status = "FAIL"
	}
	fmt.Printf("[%3d/%d] [%s] %5dms  %s\n", n, total, status, r.LatencyMS, r.Query)
	if r.Err != nil {
		fmt.Printf("        error: %v\n", r.Err)
		return
	}
	var fStr string
	if len(r.Plan.Filters) > 0 {
		parts := make([]string, 0, len(r.Plan.Filters))
		for _, f := range r.Plan.Filters {
			parts = append(parts, f.Field+"=["+strings.Join(f.Values, ",")+"]")
		}
		fStr = " filters=" + strings.Join(parts, " ")
	}
	fmt.Printf("        plan: report=%s class=%s method=%s period=%s%s\n",
		r.Plan.Report, r.Plan.Class, r.Plan.Method, r.Plan.Period.Token, fStr)
	for _, m := range r.Mismatch {
		fmt.Printf("        ✗ %s\n", m)
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
			return nil, fmt.Errorf("line %q: %w", string(line), err)
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
