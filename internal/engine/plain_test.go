package engine

import (
	"testing"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/plan"
)

func TestPlainTotalsDoNotSumAverages(t *testing.T) {
	rep := paymentReport(t)
	p := plan.AnalysisPlan{
		Report:  "payment",
		Metrics: []string{"sum_all", "kol_vo_chekov", "sredniy_chek"},
		GroupBy: []string{"date"},
		Method:  "plain",
	}
	res := dooglys.Result{Rows: []dooglys.Row{
		{"date": "2026-06-18", "sum_all": 510.74, "kol_vo_chekov": 4.0, "sredniy_chek": 127.68},
		{"date": "2026-06-15", "sum_all": 416.0, "kol_vo_chekov": 1.0, "sredniy_chek": 416.0},
	}}

	e := Plain(p, rep, res, "tnt", "RUB", per())

	if e.Summary["sum_all"] != 926.74 {
		t.Errorf("sum_all = %v, want 926.74", e.Summary["sum_all"])
	}
	if e.Summary["kol_vo_chekov"] != 5 {
		t.Errorf("kol_vo_chekov = %v, want 5", e.Summary["kol_vo_chekov"])
	}
	// Средний чек = выручка/чеки = 926.74/5 = 185.35, а НЕ сумма средних (543.68).
	if e.Summary["sredniy_chek"] != 185.35 {
		t.Errorf("sredniy_chek = %v, want 185.35 (взвешенный, не сумма средних)", e.Summary["sredniy_chek"])
	}
}
