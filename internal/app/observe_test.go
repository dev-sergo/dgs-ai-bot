package app

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"dgsbot/internal/catalog"
	"dgsbot/internal/plan"
	"dgsbot/internal/resolver"
)

// captureHandler — минимальный slog.Handler, складывающий записи для проверок.
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

func (h *captureHandler) find(msg string) (map[string]string, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message != msg {
			continue
		}
		attrs := map[string]string{}
		r.Attrs(func(a slog.Attr) bool {
			attrs[a.Key] = a.Value.String()
			return true
		})
		return attrs, true
	}
	return nil, false
}

func (h *captureHandler) count(msg string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Message == msg {
			n++
		}
	}
	return n
}

// TestApp_RefineLogsChanges: logAsk пишет только финальный план, поэтому переписи
// refine не видно. planner.refine_changed должен фиксировать before/after и только
// при реальном изменении (иначе шум на каждый запрос).
func TestApp_RefineLogsChanges(t *testing.T) {
	cap := &captureHandler{}
	a := &App{Logger: slog.New(cap)}

	before := plan.AnalysisPlan{Intent: "report", Report: "products", Method: "plain"}
	after := plan.AnalysisPlan{Intent: "report", Report: "products", Method: "top_n"}
	a.logRefineDiff("топ товаров", before, after)

	attrs, ok := cap.find("planner.refine_changed")
	if !ok {
		t.Fatalf("нет записи planner.refine_changed; записи: %+v", cap.records)
	}
	if attrs["method_before"] != "plain" || attrs["method_after"] != "top_n" {
		t.Errorf("method before/after неверны: %+v", attrs)
	}
	if attrs["query"] != "топ товаров" {
		t.Errorf("query=%q", attrs["query"])
	}

	// Без изменений — записи быть не должно.
	same := plan.AnalysisPlan{Intent: "report", Report: "payment", Method: "plain"}
	a.logRefineDiff("выручка", same, same)
	if cap.count("planner.refine_changed") != 1 {
		t.Errorf("ожидалась ровно 1 запись refine_changed, получено %d", cap.count("planner.refine_changed"))
	}
}

// TestApp_ResolverMissLogsWarn: not_found-промах имя→uuid в outcome-логе слит в
// clarify; resolver.miss должен дать отдельный структурный сигнал (kind/name/type).
func TestApp_ResolverMissLogsWarn(t *testing.T) {
	cap := &captureHandler{}
	a := &App{cat: catalog.Default(), Logger: slog.New(cap)}

	rep, ok := a.cat.Report("products")
	if !ok {
		t.Fatal("нет отчёта products в каталоге")
	}
	pfs := []plan.Filter{{Field: "product", Values: []string{"несуществующий товар"}}}
	_, clarify := a.resolveFilters(&resolver.Store{}, rep, pfs)
	if clarify == "" {
		t.Fatal("ожидался текст уточнения при ненайденном товаре")
	}

	attrs, ok := cap.find("resolver.miss")
	if !ok {
		t.Fatalf("нет записи resolver.miss; записи: %+v", cap.records)
	}
	if attrs["kind"] != "product" {
		t.Errorf("kind=%q, ожидалось product", attrs["kind"])
	}
	if attrs["type"] != "not_found" {
		t.Errorf("type=%q, ожидалось not_found", attrs["type"])
	}
	if attrs["name"] != "несуществующий товар" {
		t.Errorf("name=%q", attrs["name"])
	}
}
