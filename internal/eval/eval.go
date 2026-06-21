// Package eval — бенчмарк качества планировщика: прогон набора запросов через
// реальную модель и сверка получившегося AnalysisPlan с ожиданиями.
//
// Ловит системно то, что ручная проверка ловит случайно: модель не заполнила
// group_by/method, выбрала не тот отчёт/период, потеряла фильтр и т.п.
package eval

import (
	"context"
	"sort"
	"strings"
	"time"

	"dgsbot/internal/catalog"
	"dgsbot/internal/engine"
	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
)

// Expect — ожидаемые свойства плана (проверяются только заданные поля).
type Expect struct {
	Intent      string   `json:"intent,omitempty"` // report|help|smalltalk|off_topic
	Report      string   `json:"report,omitempty"`
	Class       string   `json:"class,omitempty"`
	Method      string   `json:"method,omitempty"`
	PeriodToken string   `json:"period_token,omitempty"`
	Filters     []string `json:"filters,omitempty"` // имена фильтров, которые должны присутствовать
}

// Case — один кейс набора.
type Case struct {
	Query  string `json:"query"`
	Expect Expect `json:"expect"`
}

// Result — итог по кейсу.
type Result struct {
	Query     string
	Plan      plan.AnalysisPlan
	Valid     bool
	Mismatch  []string
	LatencyMS int64
	Err       error
}

// Pass — кейс прошёл (нет ошибки, план валиден, нет расхождений).
func (r Result) Pass() bool { return r.Err == nil && r.Valid && len(r.Mismatch) == 0 }

// Check сверяет план с ожиданиями; возвращает список расхождений.
func Check(p plan.AnalysisPlan, e Expect) []string {
	var m []string
	add := func(s string) { m = append(m, s) }

	// Ожидания могут содержать альтернативы через "|" (напр. "this_month|last_30_days").
	if e.Intent != "" && !matchAlt(p.EffectiveIntent(), e.Intent) {
		add("intent=" + p.EffectiveIntent() + " ожидался " + e.Intent)
	}
	if e.Report != "" && !matchAlt(p.Report, e.Report) {
		add("report=" + p.Report + " ожидался " + e.Report)
	}
	if e.Class != "" && !matchAlt(string(p.Class), e.Class) {
		add("class=" + string(p.Class) + " ожидался " + e.Class)
	}
	if e.Method != "" && !matchAlt(p.Method, e.Method) {
		add("method=" + p.Method + " ожидался " + e.Method)
	}
	if e.PeriodToken != "" && !matchAlt(p.Period.Token, e.PeriodToken) {
		add("period=" + p.Period.Token + " ожидался " + e.PeriodToken)
	}
	for _, want := range e.Filters {
		if !hasFilter(p.Filters, want) {
			add("нет фильтра " + want)
		}
	}
	return m
}

// matchAlt сверяет значение со спецификацией, где варианты разделены "|".
func matchAlt(got, spec string) bool {
	for _, opt := range strings.Split(spec, "|") {
		if got == strings.TrimSpace(opt) {
			return true
		}
	}
	return false
}

func hasFilter(fs []plan.Filter, field string) bool {
	for _, f := range fs {
		if f.Field == field {
			return true
		}
	}
	return false
}

// Параметры устойчивости прогона: таймаут на ОДИН запрос и ретраи на сетевой сбой.
// Глобального дедлайна на весь набор нет — длинные прогоны не должны падать в хвосте.
const (
	perCallTimeout = 90 * time.Second
	maxAttempts    = 3
	retryBackoff   = 2 * time.Second
)

// Run прогоняет все кейсы через планировщик и валидатор.
func Run(ctx context.Context, pl planner.Planner, cat *catalog.Catalog, cases []Case) []Result {
	out := make([]Result, 0, len(cases))
	for _, c := range cases {
		start := time.Now()
		p, err := planWithRetry(ctx, pl, c.Query)
		lat := time.Since(start).Milliseconds()

		r := Result{Query: c.Query, Plan: p, LatencyMS: lat, Err: err}
		if err == nil {
			// Та же нормализация, что в проде (app.go), — бенчмарк должен мерить
			// итоговый план, а не сырой ответ модели.
			engine.NormalizeMethod(&p)
			r.Plan = p
			if p.IsReport() {
				val := plan.Validate(&p, cat)
				r.Valid = val.OK || val.NeedClarify
			} else {
				r.Valid = true // help/smalltalk/off_topic — валидны как разговорный ответ
			}
			r.Mismatch = Check(p, c.Expect)
		}
		out = append(out, r)
	}
	return out
}

// planWithRetry дёргает планировщик с таймаутом на запрос и ретраями на ошибку
// (таймаут рига, разрыв сети). Между попытками — небольшой бэкофф.
func planWithRetry(parent context.Context, pl planner.Planner, query string) (plan.AnalysisPlan, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(parent, perCallTimeout)
		p, err := pl.Plan(ctx, nil, query) // eval — без истории диалога
		cancel()
		if err == nil {
			return p, nil
		}
		lastErr = err
		if parent.Err() != nil { // родительский контекст отменён — дальше нет смысла
			break
		}
		if attempt < maxAttempts {
			select {
			case <-time.After(retryBackoff):
			case <-parent.Done():
			}
		}
	}
	return plan.AnalysisPlan{}, lastErr
}

// Stats — сводка по прогону.
type Stats struct {
	Total  int
	Passed int
	Valid  int
	Errors int
	LatP50 int64
	LatP95 int64
	LatMax int64
}

// Summarize считает агрегаты по результатам.
func Summarize(rs []Result) Stats {
	s := Stats{Total: len(rs)}
	lats := make([]int64, 0, len(rs))
	for _, r := range rs {
		if r.Err != nil {
			s.Errors++
		}
		if r.Valid {
			s.Valid++
		}
		if r.Pass() {
			s.Passed++
		}
		lats = append(lats, r.LatencyMS)
	}
	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	s.LatP50 = percentile(lats, 50)
	s.LatP95 = percentile(lats, 95)
	if len(lats) > 0 {
		s.LatMax = lats[len(lats)-1]
	}
	return s
}

func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
