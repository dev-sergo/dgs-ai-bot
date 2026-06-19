package engine

import (
	"testing"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

func paymentReport(t *testing.T) catalog.Report {
	t.Helper()
	r, ok := catalog.Default().Report("payment")
	if !ok {
		t.Fatal("нет отчёта payment в каталоге")
	}
	return r
}

func per() envelope.Period { return envelope.Period{From: "01.06.2026", To: "07.06.2026", TZ: "Europe/Moscow"} }

func TestCompare(t *testing.T) {
	rep := paymentReport(t)
	now := dooglys.Result{Rows: []dooglys.Row{{"sum_all": 100.0}, {"sum_all": 50.0}}}
	prev := dooglys.Result{Rows: []dooglys.Row{{"sum_all": 100.0}}}

	e := Compare(rep, "sum_all", now, prev, "tnt", "RUB", per(), per())
	if e.Summary["value_now"] != 150 || e.Summary["value_prev"] != 100 {
		t.Fatalf("суммы периодов неверны: %+v", e.Summary)
	}
	if e.Summary["delta_abs"] != 50 || e.Summary["delta_pct"] != 50 {
		t.Fatalf("дельта неверна: %+v", e.Summary)
	}
}

func TestContributionDecomposesDelta(t *testing.T) {
	rep := paymentReport(t)
	// now: card90 cash30 → 120; prev: card50 cash50 → 100; total delta 20.
	now := dooglys.Result{Rows: []dooglys.Row{{"sum_card": 90.0, "sum_cash": 30.0, "onlayn": 0.0, "sbp": 0.0, "sum_all": 120.0}}}
	prev := dooglys.Result{Rows: []dooglys.Row{{"sum_card": 50.0, "sum_cash": 50.0, "onlayn": 0.0, "sbp": 0.0, "sum_all": 100.0}}}

	e := Contribution(rep, "sum_all", now, prev, 5, "tnt", "RUB", per(), per())
	if e.Summary["delta_abs"] != 20 {
		t.Fatalf("total delta = %v, want 20", e.Summary["delta_abs"])
	}
	// Сумма вкладов компонент должна равняться общему изменению.
	var sum float64
	for _, r := range e.Rows {
		sum += r["delta"].(float64)
	}
	if sum != 20 {
		t.Errorf("сумма вкладов = %v, want 20 (раскладка должна быть точной)", sum)
	}
	// Первая строка — наибольшее по модулю изменение (карта +40).
	if e.Rows[0]["component"] != "Карта" || e.Rows[0]["delta"].(float64) != 40 {
		t.Errorf("ожидался топ-вклад Карта +40, got %v / %v", e.Rows[0]["component"], e.Rows[0]["delta"])
	}
}
