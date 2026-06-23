package engine

import (
	"testing"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

func TestBuildInsightBundle(t *testing.T) {
	paymentNow := dooglys.Result{Rows: []dooglys.Row{
		{"sum_all": 1000.0, "return_sum": 150.0, "return_count": 2.0},
		{"sum_all": 500.0, "return_sum": 50.0, "return_count": 1.0},
	}}
	paymentPrev := dooglys.Result{Rows: []dooglys.Row{
		{"sum_all": 2000.0, "return_sum": 100.0, "return_count": 1.0},
	}}
	productsNow := dooglys.Result{Rows: []dooglys.Row{
		{"name": "Пицца", "quantity": 10.0, "amount": 5000.0, "profit": 3000.0, "discount_sum": 100.0},
		{"name": "Вода", "quantity": 3.0, "amount": 90.0, "profit": -10.0, "discount_sum": 5.0},
		{"name": "Вода", "quantity": 2.0, "amount": 60.0, "profit": -5.0, "discount_sum": 0.0}, // дубль — агрегируется
		{"name": "Салфетка", "quantity": 0.0, "amount": 0.0, "profit": 0.0, "discount_sum": 0.0}, // не продавалось — выкидываем
	}}

	period := envelope.Period{From: "2026-06-01", To: "2026-06-07"}
	prev := envelope.Period{From: "2026-05-25", To: "2026-05-31"}
	b := BuildInsightBundle(paymentNow, paymentPrev, productsNow, "RUB", period, prev)

	// Выручка: 1500 текущий vs 2000 предыдущий → -500, -25%
	if b.Revenue.Now != 1500 || b.Revenue.Prev != 2000 || b.Revenue.DeltaAbs != -500 {
		t.Errorf("Revenue = %+v, want now=1500 prev=2000 delta=-500", b.Revenue)
	}
	if b.Revenue.DeltaPct != -25 {
		t.Errorf("Revenue.DeltaPct = %v, want -25", b.Revenue.DeltaPct)
	}
	// Возвраты: 200 текущий, 3 штуки
	if b.ReturnsSum.Now != 200 || b.ReturnCount != 3 {
		t.Errorf("Returns: sum=%v count=%v, want 200/3", b.ReturnsSum.Now, b.ReturnCount)
	}
	// Скидки: 105
	if b.Discounts != 105 {
		t.Errorf("Discounts = %v, want 105", b.Discounts)
	}
	// Аутсайдеры: «Вода» (агрегат 150₽, 5 шт, прибыль -15) первой; «Салфетка» отброшена (0 продаж)
	if len(b.BottomProducts) != 2 {
		t.Fatalf("BottomProducts: %d, want 2 (Салфетка с 0 продаж исключена): %+v", len(b.BottomProducts), b.BottomProducts)
	}
	water := b.BottomProducts[0]
	if water.Name != "Вода" || water.Amount != 150 || water.Quantity != 5 || water.Profit != -15 {
		t.Errorf("первый аутсайдер = %+v, want Вода amount=150 qty=5 profit=-15", water)
	}
}

// Пустой предыдущий период → EmptyPrev=true, проценты не считаются.
func TestBuildInsightBundle_EmptyPrev(t *testing.T) {
	paymentNow := dooglys.Result{Rows: []dooglys.Row{{"sum_all": 1000.0}}}
	b := BuildInsightBundle(paymentNow, dooglys.Result{}, dooglys.Result{}, "RUB",
		envelope.Period{}, envelope.Period{})
	if !b.Revenue.EmptyPrev || b.Revenue.DeltaPct != 0 {
		t.Errorf("при пустом prev ждём EmptyPrev=true и DeltaPct=0, got %+v", b.Revenue)
	}
}
