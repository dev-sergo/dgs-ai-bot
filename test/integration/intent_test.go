package integration

import (
	"context"
	"strings"
	"testing"

	"dgsbot/internal/planner"
)

// Не-данные запросы НЕ должны превращаться в отчёт.
func TestHelpIntentNoReport(t *testing.T) {
	a := newApp(t) // StubPlanner ловит «что умеешь»
	ans, err := a.Ask(context.Background(), "mock_single", "s", "какие у тебя функции?")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil {
		t.Fatal("на help-запрос не должно быть отчёта (envelope)")
	}
	if ans.Plan.EffectiveIntent() != "help" {
		t.Fatalf("intent = %s, want help", ans.Plan.EffectiveIntent())
	}
	if !strings.Contains(ans.Text, "Выручка") || !strings.Contains(ans.Text, "Спросите") {
		t.Errorf("help-ответ должен перечислять возможности:\n%s", ans.Text)
	}
}

func TestSmalltalkIntent(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "привет")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil || ans.Plan.EffectiveIntent() != "smalltalk" {
		t.Fatalf("ожидался smalltalk без отчёта: intent=%s", ans.Plan.EffectiveIntent())
	}
}

func TestOffTopicIntent(t *testing.T) {
	a := newApp(t)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "расскажи анекдот")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil || ans.Plan.EffectiveIntent() != "off_topic" {
		t.Fatalf("ожидался off_topic без отчёта: intent=%s", ans.Plan.EffectiveIntent())
	}
}

// Память диалога накапливается по сессии.
func TestSessionMemoryAccumulates(t *testing.T) {
	a, store := newAppStore(t, planner.NewStub())
	_, _ = a.Ask(context.Background(), "mock_single", "sess1", "выручка за неделю")
	_, _ = a.Ask(context.Background(), "mock_single", "sess1", "почему упала выручка за месяц")

	if got := len(store.History("sess1")); got != 4 {
		t.Errorf("ожидалось 4 реплики в истории, got %d", got)
	}
	if len(store.History("sess2")) != 0 {
		t.Error("чужая сессия не должна видеть историю")
	}
}
