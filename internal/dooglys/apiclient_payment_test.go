package dooglys

import "testing"

func ret(ts int64) *int64 { return &ts }

// sampleOrders — синтетика, повторяющая форму /sales/order/list.
// Правило: выручка = processing_status "finished", total_cost, возвраты вычитаются.
func sampleOrders() []order {
	return []order{
		// День 2025-05-06: 2 оплаченных чека (cash + card), 1 возврат (online).
		{SalePointID: "sp1", TotalCost: 700, ItemsCost: 300, PaymentType: "cash", ProcessingStatus: "finished", CashierShiftDate: "2025-05-06"},
		{SalePointID: "sp1", TotalCost: 500, ItemsCost: 500, PaymentType: "card", ProcessingStatus: "finished", CashierShiftDate: "2025-05-06"},
		{SalePointID: "sp2", TotalCost: 200, ItemsCost: 200, PaymentType: "online", ProcessingStatus: "canceled", CashierShiftDate: "2025-05-06", DateReturned: ret(1746600000)},
		// confirmed (не finished) — не выручка и не возврат.
		{SalePointID: "sp1", TotalCost: 999, ItemsCost: 999, PaymentType: "cash", ProcessingStatus: "confirmed", CashierShiftDate: "2025-05-06"},
		// День 2025-05-07: 1 чек sbp.
		{SalePointID: "sp2", TotalCost: 1000, ItemsCost: 900, PaymentType: "sbp", ProcessingStatus: "finished", CashierShiftDate: "2025-05-07"},
		// Вне периода — отсекается.
		{SalePointID: "sp1", TotalCost: 123, PaymentType: "cash", ProcessingStatus: "finished", CashierShiftDate: "2025-05-09"},
	}
}

func rowByDate(rows []Row, d string) Row {
	for _, r := range rows {
		if r["date"] == d {
			return r
		}
	}
	return nil
}

func TestAggregatePayment_Basic(t *testing.T) {
	rows, _, _ := aggregatePayment(sampleOrders(), "2025-05-06", "2025-05-07", nil, DefaultPaymentRule())
	if len(rows) != 2 {
		t.Fatalf("ожидали 2 дня, получили %d: %+v", len(rows), rows)
	}

	d6 := rowByDate(rows, "2025-05-06")
	if d6 == nil {
		t.Fatal("нет строки 2025-05-06")
	}
	// 2 чека (cash 700 + card 500); online — возврат (200, вычитается); confirmed — мимо.
	if got := d6["kol_vo_chekov"].(float64); got != 2 {
		t.Errorf("kol_vo_chekov=%v, want 2", got)
	}
	if got := d6["sum_all"].(float64); got != 1000 { // 1200 валовая − 200 возврат
		t.Errorf("sum_all=%v, want 1000 (net)", got)
	}
	if got := d6["sum_cash"].(float64); got != 700 {
		t.Errorf("sum_cash=%v, want 700", got)
	}
	if got := d6["sum_card"].(float64); got != 500 {
		t.Errorf("sum_card=%v, want 500", got)
	}
	if got := d6["return_count"].(float64); got != 1 {
		t.Errorf("return_count=%v, want 1", got)
	}
	if got := d6["return_sum"].(float64); got != 200 {
		t.Errorf("return_sum=%v, want 200", got)
	}
	if got := d6["sredniy_chek"].(float64); got != 500 { // net 1000 / 2 чека
		t.Errorf("sredniy_chek=%v, want 500", got)
	}
}

// Каналы оплаты после вычета возвратов сходятся с чистой выручкой строки.
func TestAggregatePayment_ChannelsReconcile(t *testing.T) {
	rows, _, _ := aggregatePayment(sampleOrders(), "2025-05-06", "2025-05-07", nil, DefaultPaymentRule())
	for _, r := range rows {
		channels := r["sum_cash"].(float64) + r["sum_card"].(float64) + r["onlayn"].(float64) + r["sbp"].(float64)
		if round2(channels) != r["sum_all"].(float64) {
			t.Errorf("день %v: каналы %.2f != выручка %.2f", r["date"], channels, r["sum_all"])
		}
	}
}

func TestAggregatePayment_PeriodCutoff(t *testing.T) {
	rows, _, _ := aggregatePayment(sampleOrders(), "2025-05-06", "2025-05-06", nil, DefaultPaymentRule())
	if len(rows) != 1 || rows[0]["date"] != "2025-05-06" {
		t.Fatalf("ожидали только 2025-05-06, получили %+v", rows)
	}
}

func TestAggregatePayment_WithoutDelivery(t *testing.T) {
	rule := DefaultPaymentRule()
	rule.WithDelivery = false // считаем items_cost
	rows, _, _ := aggregatePayment(sampleOrders(), "2025-05-06", "2025-05-06", nil, rule)
	d6 := rowByDate(rows, "2025-05-06")
	if got := d6["sum_all"].(float64); got != 600 { // (300 + 500) − 200 возврат
		t.Errorf("items_cost sum_all=%v, want 600", got)
	}
}

func TestAggregatePayment_FilterSalePoint(t *testing.T) {
	f := []QueryFilter{{Field: "sale_point", Param: "sale_point_id", UUIDs: []string{"sp2"}}}
	rows, applied, _ := aggregatePayment(sampleOrders(), "2025-05-06", "2025-05-07", f, DefaultPaymentRule())
	if len(applied) != 1 || applied[0] != "sale_point" {
		t.Errorf("applied=%v, want [sale_point]", applied)
	}
	// sp2: 2025-05-06 только возврат online; 2025-05-07 — sbp чек.
	d7 := rowByDate(rows, "2025-05-07")
	if d7 == nil || d7["sbp"].(float64) != 1000 {
		t.Errorf("ожидали sp2 sbp=1000 на 2025-05-07, получили %+v", rows)
	}
	if d6 := rowByDate(rows, "2025-05-06"); d6 != nil && d6["kol_vo_chekov"].(float64) != 0 {
		t.Errorf("у sp2 на 2025-05-06 не должно быть чеков (только возврат): %+v", d6)
	}
}

func TestAggregatePayment_FilterPaymentType(t *testing.T) {
	f := []QueryFilter{{Field: "payment_type", Param: "payment_type", Names: []string{"card"}}}
	rows, applied, _ := aggregatePayment(sampleOrders(), "2025-05-06", "2025-05-07", f, DefaultPaymentRule())
	if len(applied) != 1 {
		t.Fatalf("applied=%v", applied)
	}
	d6 := rowByDate(rows, "2025-05-06")
	if d6 == nil || d6["kol_vo_chekov"].(float64) != 1 || d6["sum_card"].(float64) != 500 {
		t.Errorf("ожидали 1 card-чек на 500, получили %+v", d6)
	}
}

func TestIsoRange(t *testing.T) {
	from, to, err := isoRange("06.05.2025", "07.05.2025")
	if err != nil || from != "2025-05-06" || to != "2025-05-07" {
		t.Fatalf("isoRange = %q,%q,%v", from, to, err)
	}
}
