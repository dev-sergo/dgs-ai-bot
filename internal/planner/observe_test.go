package planner

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"dgsbot/internal/config"
	"dgsbot/internal/llm"
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

// find возвращает первую запись с заданным message и map её атрибутов.
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

// TestLLMPlanner_ParseFailLogsSnippet: битый (не-JSON) ответ модели маскируется под
// smalltalk в outcome-логе; новый сигнал planner.parse_fail должен нести причину и
// обрезанный raw, чтобы отличить мусор модели от настоящей болтовни.
func TestLLMPlanner_ParseFailLogsSnippet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Валидный конверт чата, но content — не JSON плана → parsePlan фейлится.
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"это не json"}}]}`))
	}))
	defer srv.Close()

	cli := llm.New(config.LLM{BaseURL: srv.URL, Timeout: 5 * time.Second})
	cap := &captureHandler{}
	p := NewLLM(cli, "test-model", false)
	p.logger = slog.New(cap)

	pl, err := p.Plan(context.Background(), nil, "спасибо")
	if err != nil {
		t.Fatalf("Plan вернул ошибку: %v", err)
	}
	if pl.Intent != "smalltalk" {
		t.Fatalf("ожидался fail-closed intent=smalltalk, получено %q", pl.Intent)
	}

	attrs, ok := cap.find("planner.parse_fail")
	if !ok {
		t.Fatalf("нет записи planner.parse_fail; записи: %+v", cap.records)
	}
	if attrs["raw"] == "" {
		t.Errorf("ожидался непустой атрибут raw, получено %+v", attrs)
	}
	if attrs["err"] == "" {
		t.Errorf("ожидался непустой атрибут err, получено %+v", attrs)
	}
	if attrs["query"] != "спасибо" {
		t.Errorf("query=%q, ожидалось «спасибо»", attrs["query"])
	}
}
