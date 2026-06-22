package engine

import (
	"testing"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dooglys"
)

func productsReport(t *testing.T) catalog.Report {
	t.Helper()
	r, ok := catalog.Default().Report("products")
	if !ok {
		t.Fatal("нет отчёта products в каталоге")
	}
	return r
}

func TestProductsSupportContribution(t *testing.T) {
	if !SupportsContribution("products") {
		t.Fatal("products должен поддерживать contribution (раскладка по товарам)")
	}
}

// «Какой товар виноват в падении выручки» — вклад по товарам (name через amount).
func TestContributionByProduct(t *testing.T) {
	rep := productsReport(t)
	// Кофе: 100→40 (−60), Чай: 50→80 (+30), Сок: 0→10 (+10). Итого 150→130 = −20.
	now := dooglys.Result{Rows: []dooglys.Row{
		{"name": "Кофе", "amount": 40.0},
		{"name": "Чай", "amount": 80.0},
		{"name": "Сок", "amount": 10.0},
	}}
	prev := dooglys.Result{Rows: []dooglys.Row{
		{"name": "Кофе", "amount": 100.0},
		{"name": "Чай", "amount": 50.0},
	}}

	e := Contribution(rep, "amount", now, prev, 5, "tnt", "RUB", per(), per())

	if e.Summary["delta_abs"] != -20 {
		t.Fatalf("общая дельта = %v, want -20", e.Summary["delta_abs"])
	}
	// Топ движения по модулю — Кофе (−60).
	if e.Rows[0]["component"] != "Кофе" || e.Rows[0]["delta"].(float64) != -60 {
		t.Errorf("ожидался топ-вклад Кофе −60, got %v / %v", e.Rows[0]["component"], e.Rows[0]["delta"])
	}
	// Сумма вкладов товаров = общему изменению (раскладка точная).
	var sum float64
	for _, r := range e.Rows {
		sum += r["delta"].(float64)
	}
	if round2(sum) != -20 {
		t.Errorf("сумма вкладов = %v, want -20", sum)
	}
	// Первая колонка — измерение «Название», а не фиксированная «Компонента».
	if e.Columns[0].Label != "Название" {
		t.Errorf("первая колонка = %q, want Название", e.Columns[0].Label)
	}
}
