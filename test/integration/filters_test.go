package integration

import (
	"context"
	"testing"

	"dgsbot/internal/plan"
	"dgsbot/internal/session"
)

// fakePlanner отдаёт заранее заданный план (для проверки фильтров/PII независимо от LLM).
type fakePlanner struct{ p plan.AnalysisPlan }

func (f fakePlanner) Plan(_ context.Context, _ []session.Message, _ string) (plan.AnalysisPlan, error) {
	return f.p, nil
}

func ordersPlan(filters ...plan.Filter) plan.AnalysisPlan {
	return plan.AnalysisPlan{
		Version: "1", Class: plan.ClassA, Report: "orders",
		Metrics: []string{"paid"}, GroupBy: []string{"torgovaya_tochka"},
		Period: plan.Period{Kind: "relative", Token: "last_30_days"},
		Method: "plain", Output: plan.Output{Format: "text"}, Filters: filters,
	}
}

func TestOrdersFilteredBySalePoint(t *testing.T) {
	a := newAppWith(t, fakePlanner{ordersPlan(
		plan.Filter{Field: "sale_point", Op: "in", Values: []string{"Выкса"}},
	)})
	ans, err := a.Ask(context.Background(), "mock_single", "s", "заказы по точке Выкса")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope: %+v", ans.Validation)
	}
	if len(ans.Envelope.Rows) == 0 {
		t.Fatal("ожидались строки после фильтра по точке")
	}
	for _, r := range ans.Envelope.Rows {
		if r["torgovaya_tochka"] != "Выкса" {
			t.Errorf("после фильтра встретилась точка %v, ожидалась только Выкса", r["torgovaya_tochka"])
		}
	}
	if !containsStr(metaStrings(ans.Envelope.Meta["filters_applied"]), "sale_point") {
		t.Errorf("ожидался применённый фильтр sale_point, meta=%v", ans.Envelope.Meta)
	}
}

func TestPIINotExposed(t *testing.T) {
	a := newAppWith(t, fakePlanner{ordersPlan()})
	ans, err := a.Ask(context.Background(), "mock_single", "s", "заказы")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope")
	}
	for _, c := range ans.Envelope.Columns {
		if c.Key == "kassir" || c.Key == "pokupatel" {
			t.Errorf("PII-колонка %q просочилась в результат", c.Key)
		}
	}
	for _, r := range ans.Envelope.Rows {
		if _, ok := r["kassir"]; ok {
			t.Error("PII-поле kassir есть в строке результата")
		}
	}
}

func TestUnresolvedFilterAsksClarify(t *testing.T) {
	a := newAppWith(t, fakePlanner{ordersPlan(
		plan.Filter{Field: "sale_point", Op: "in", Values: []string{"Несуществующая Точка"}},
	)})
	ans, err := a.Ask(context.Background(), "mock_single", "s", "заказы по точке X")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Validation.OK || !ans.Validation.NeedClarify {
		t.Fatalf("ожидалось уточнение по нерезолвнутой точке: %+v", ans.Validation)
	}
	if ans.Envelope != nil {
		t.Error("envelope не должен формироваться при нерезолвнутом фильтре")
	}
}

// Модель может не вернуть group_by — оркестратор обязан подставить дефолтное измерение.
func TestDefaultDimensionInjected(t *testing.T) {
	p := plan.AnalysisPlan{
		Version: "1", Class: plan.ClassA, Report: "payment",
		Metrics: []string{"sum_all"}, // group_by НЕ задан
		Period: plan.Period{Kind: "relative", Token: "last_7_days"},
		Method: "plain", Output: plan.Output{Format: "text"},
	}
	a := newAppWith(t, fakePlanner{p})
	ans, err := a.Ask(context.Background(), "mock_single", "s", "выручка за неделю")
	if err != nil {
		t.Fatal(err)
	}
	if ans.Envelope == nil {
		t.Fatalf("нет envelope: %+v", ans.Validation)
	}
	hasDate := false
	for _, c := range ans.Envelope.Columns {
		if c.Key == "date" {
			hasDate = true
		}
	}
	if !hasDate {
		t.Errorf("ожидалась колонка date (дефолтное измерение), got columns=%+v", ans.Envelope.Columns)
	}
}

func metaStrings(v any) []string {
	if ss, ok := v.([]string); ok {
		return ss
	}
	return nil
}

func containsStr(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
