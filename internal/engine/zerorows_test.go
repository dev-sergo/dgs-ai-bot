package engine

import (
	"testing"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/plan"
)

// Строки, где все метрики = 0 (день без движения), не должны попадать в отчёт.
func TestPlainDropsAllZeroRows(t *testing.T) {
	rep := paymentReport(t)
	p := plan.AnalysisPlan{
		Report:  "payment",
		Metrics: []string{"sum_all", "kol_vo_chekov"},
		GroupBy: []string{"date"},
		Method:  "plain",
	}
	res := dooglys.Result{Rows: []dooglys.Row{
		{"date": "2026-06-18", "sum_all": 510.0, "kol_vo_chekov": 4.0},
		{"date": "2026-06-17", "sum_all": 0.0, "kol_vo_chekov": 0.0}, // пустой день — шум
		{"date": "2026-06-15", "sum_all": 416.0, "kol_vo_chekov": 1.0},
	}}

	e := Plain(p, rep, res, "tnt", "RUB", per())

	if len(e.Rows) != 2 {
		t.Fatalf("ожидали 2 непустые строки, got %d: %+v", len(e.Rows), e.Rows)
	}
	for _, r := range e.Rows {
		if r["date"] == "2026-06-17" {
			t.Errorf("пустой день 2026-06-17 не должен попасть в отчёт")
		}
	}
	// Итоги по всей выборке не меняются (пустой день и так = 0).
	if e.Summary["sum_all"] != 926 {
		t.Errorf("sum_all summary = %v, want 926", e.Summary["sum_all"])
	}
}
