package integration

import (
	"context"
	"testing"

	"dgsbot/internal/plan"
)

// Вклад по товарам end-to-end: план products+contribution НЕ понижается до compare
// (товары теперь поддерживают раскладку), а в ответе — разбивка по товарам.
func TestProductContributionByName(t *testing.T) {
	p := plan.AnalysisPlan{
		Version: "1", Class: plan.ClassB, Report: "products",
		Metrics:   []string{"amount"},
		Period:    plan.Period{Kind: "relative", Token: "this_month"},
		CompareTo: &plan.Period{Kind: "relative", Token: "prev_period"},
		Method:    "contribution", TopN: 5,
		Output: plan.Output{Format: "text"}, Confidence: 0.9,
	}
	a := newAppWith(t, fakePlanner{p})
	ans, err := a.Ask(context.Background(), "mock_single", "s", "какой товар виноват в падении выручки за месяц")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope: %+v", ans.Validation)
	}
	if m, _ := ans.Envelope.Meta["method"].(string); m != "contribution" {
		t.Errorf("метод = %q, должен остаться contribution (товары поддерживают раскладку)", m)
	}
	if len(ans.Envelope.Rows) == 0 {
		t.Fatal("ожидалась разбивка по товарам, строк нет")
	}
	if ans.Envelope.Columns[0].Label != "Название" {
		t.Errorf("первая колонка = %q, ожидалась Название (разбивка по товарам)", ans.Envelope.Columns[0].Label)
	}
}
