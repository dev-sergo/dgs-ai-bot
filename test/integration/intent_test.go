package integration

import (
	"context"
	"strings"
	"testing"

	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
	"dgsbot/internal/session"
)

// fixedPlanner всегда возвращает заранее заданный план — для проверки гейтов.
type fixedPlanner struct{ p plan.AnalysisPlan }

func (f fixedPlanner) Plan(_ context.Context, _ []session.Message, _ string) (plan.AnalysisPlan, error) {
	return f.p, nil
}

// Низкая уверенность планировщика → переспрос, а не заглушка-отчёт.
func TestLowConfidenceAsksClarify(t *testing.T) {
	pl := fixedPlanner{p: plan.AnalysisPlan{
		Version: "1", Intent: "report", Class: plan.ClassA,
		Report: "payment", Metrics: []string{"sum_all"},
		GroupBy: []string{"date"},
		Period:  plan.Period{Kind: "relative", Token: "this_month"},
		Method:  "plain", Output: plan.Output{Format: "text"},
		Confidence: 0.3,
	}}
	a := newAppWith(t, pl)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "что-то непонятное")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil {
		t.Fatal("при низкой уверенности отчёт строить нельзя")
	}
	if !ans.Validation.NeedClarify {
		t.Fatalf("ожидался NeedClarify, got %+v", ans.Validation)
	}
	if !strings.Contains(ans.Text, "Не уверен") {
		t.Errorf("ожидался переспрос, got: %s", ans.Text)
	}
}

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

// Пустой период → честный ответ, а не таблица-заглушка.
func TestEmptyPeriodIsHonest(t *testing.T) {
	pl := fixedPlanner{p: plan.AnalysisPlan{
		Version: "1", Intent: "report", Class: plan.ClassA,
		Report: "payment", Metrics: []string{"sum_all"}, GroupBy: []string{"date"},
		Period: plan.Period{Kind: "explicit", From: "01.01.2000", To: "31.01.2000"},
		Method: "plain", Output: plan.Output{Format: "text"}, Confidence: 0.9,
	}}
	a := newAppWith(t, pl)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "выручка")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ans.Text, "данных для отчёта нет") {
		t.Errorf("ожидался честный ответ о пустоте:\n%s", ans.Text)
	}
}

// План вне white-list (разбивка по полю, которого нет в каталоге) → честное
// «не умею», а НЕ пустой ответ. Раньше такой план возвращался с пустым Text.
func TestOutOfScopeIsHonest(t *testing.T) {
	pl := fixedPlanner{p: plan.AnalysisPlan{
		Version: "1", Intent: "report", Class: plan.ClassA,
		Report: "payment", Metrics: []string{"sum_all"},
		GroupBy: []string{"product_category"}, // у payment такого измерения нет
		Period:  plan.Period{Kind: "relative", Token: "this_month"},
		Method:  "plain", Output: plan.Output{Format: "text"}, Confidence: 0.9,
	}}
	a := newAppWith(t, pl)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "выручка по категориям")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope != nil {
		t.Fatal("на запрос вне white-list отчёт строить нельзя")
	}
	if ans.Validation.OK {
		t.Fatalf("ожидалась невалидность плана: %+v", ans.Validation)
	}
	if ans.Text == "" {
		t.Fatal("ответ не должен быть пустым — нужно честное «не умею»")
	}
	if !strings.Contains(ans.Text, "не умею") {
		t.Errorf("ожидалось честное объяснение границ, got: %s", ans.Text)
	}
}

// contribution по отчёту без раскладки понижается до compare (не выдаёт пустоту).
// paycheck не раскладывается ни по колонкам, ни по измерению (в отличие от payment/products).
func TestContributionDowngradesWhenUnsupported(t *testing.T) {
	pl := fixedPlanner{p: plan.AnalysisPlan{
		Version: "1", Intent: "report", Class: plan.ClassB,
		Report: "paycheck", Metrics: []string{"paid"},
		Period:    plan.Period{Kind: "relative", Token: "this_month"},
		CompareTo: &plan.Period{Kind: "relative", Token: "prev_period"},
		Method:    "contribution", TopN: 5,
		Output: plan.Output{Format: "text"}, Confidence: 0.9,
	}}
	a := newAppWith(t, pl)
	ans, err := a.Ask(context.Background(), "mock_single", "s", "почему так")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatal("ожидался envelope")
	}
	if m, _ := ans.Envelope.Meta["method"].(string); m != "compare" {
		t.Errorf("метод должен быть понижен до compare, got %q", m)
	}
}

// Метажалоба о диалоге не превращается в отчёт, а уточняет запрос.
func TestMetaComplaintAsksClarify(t *testing.T) {
	a := newApp(t)
	for _, q := range []string{
		"почему ты не слышишь что я спрашиваю",
		"мне кажется отчёты одинаковые",
		"почему ты приводишь список а не конкретный товар",
	} {
		ans, err := a.Ask(context.Background(), "mock_single", "s", q)
		if err != nil {
			t.Fatal(err)
		}
		if ans.Envelope != nil {
			t.Errorf("на метажалобу %q не должно быть отчёта", q)
		}
		if ans.Plan.EffectiveIntent() != "smalltalk" {
			t.Errorf("%q: intent=%s, want smalltalk", q, ans.Plan.EffectiveIntent())
		}
		if !strings.Contains(ans.Text, "Уточните") {
			t.Errorf("%q: ожидалось уточнение, got: %s", q, ans.Text)
		}
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
