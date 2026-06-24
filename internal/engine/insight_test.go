package engine

import (
	"testing"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

func TestBuildInsightBundle(t *testing.T) {
	paymentNow := dooglys.Result{Rows: []dooglys.Row{
		{"sum_all": 1000.0, "return_sum": 150.0, "return_count": 2.0, "kol_vo_chekov": 4.0,
			"sum_card": 700.0, "sum_cash": 300.0, "onlayn": 0.0, "sbp": 0.0},
		{"sum_all": 500.0, "return_sum": 50.0, "return_count": 1.0, "kol_vo_chekov": 2.0,
			"sum_card": 500.0, "sum_cash": 0.0, "onlayn": 0.0, "sbp": 0.0},
	}}
	paymentPrev := dooglys.Result{Rows: []dooglys.Row{
		{"sum_all": 2000.0, "return_sum": 100.0, "return_count": 1.0, "kol_vo_chekov": 8.0},
	}}
	productsNow := dooglys.Result{Rows: []dooglys.Row{
		{"name": "Пицца", "quantity": 10.0, "amount": 5000.0, "profit": 3000.0, "discount_sum": 100.0},
		{"name": "Вода", "quantity": 3.0, "amount": 90.0, "profit": -10.0, "discount_sum": 5.0},
		{"name": "Вода", "quantity": 2.0, "amount": 60.0, "profit": -5.0, "discount_sum": 0.0},
		{"name": "Салфетка", "quantity": 0.0, "amount": 0.0, "profit": 0.0, "discount_sum": 0.0},
	}}

	period := envelope.Period{From: "2026-06-01", To: "2026-06-07"}
	prev := envelope.Period{From: "2026-05-25", To: "2026-05-31"}
	b := BuildInsightBundle(paymentNow, paymentPrev, productsNow, "RUB", period, prev)

	// Выручка: 1500 vs 2000 → -500, -25%
	if b.Revenue.Now != 1500 || b.Revenue.Prev != 2000 || b.Revenue.DeltaAbs != -500 {
		t.Errorf("Revenue = %+v, want now=1500 prev=2000 delta=-500", b.Revenue)
	}
	if b.Revenue.DeltaPct != -25 {
		t.Errorf("Revenue.DeltaPct = %v, want -25", b.Revenue.DeltaPct)
	}

	// AvgCheck: now = 1500/6 = 250; prev = 2000/8 = 250 → delta=0
	if b.AvgCheck.Now != 250 || b.AvgCheck.Prev != 250 || b.AvgCheck.DeltaAbs != 0 {
		t.Errorf("AvgCheck = %+v, want now=250 prev=250 delta=0", b.AvgCheck)
	}

	// Возвраты: 200 текущий, 3 штуки
	if b.ReturnsSum.Now != 200 || b.ReturnCount != 3 {
		t.Errorf("Returns: sum=%v count=%v, want 200/3", b.ReturnsSum.Now, b.ReturnCount)
	}

	// Скидки: 105
	if b.Discounts != 105 {
		t.Errorf("Discounts = %v, want 105", b.Discounts)
	}

	// ReturnRate — к payment-выручке: 200/1500*100 = 13.33.
	if b.ReturnRate != 13.33 {
		t.Errorf("ReturnRate = %v, want 13.33", b.ReturnRate)
	}
	// DiscountShare — к выручке ТОВАРОВ (products.amount = 5150): 105/5150*100 = 2.04.
	// Один источник со скидками → корректно и в гибридном режиме (payment=API, products=фикстура).
	if b.DiscountShare != 2.04 {
		t.Errorf("DiscountShare = %v, want 2.04", b.DiscountShare)
	}

	// ChannelMix: Карта 1200 (80%), Наличные 300 (20%); нулевые каналы отброшены
	if len(b.ChannelMix) != 2 {
		t.Fatalf("ChannelMix len=%d, want 2: %+v", len(b.ChannelMix), b.ChannelMix)
	}
	if b.ChannelMix[0].Label != "Карта" || b.ChannelMix[0].Now != 1200 {
		t.Errorf("ChannelMix[0] = %+v, want Карта 1200", b.ChannelMix[0])
	}
	if b.ChannelMix[1].Label != "Наличные" || b.ChannelMix[1].Now != 300 {
		t.Errorf("ChannelMix[1] = %+v, want Наличные 300", b.ChannelMix[1])
	}

	// TopProducts: Пицца первая (5000), Вода вторая (150)
	if len(b.TopProducts) < 2 {
		t.Fatalf("TopProducts len=%d, want >=2: %+v", len(b.TopProducts), b.TopProducts)
	}
	if b.TopProducts[0].Name != "Пицца" || b.TopProducts[0].Amount != 5000 {
		t.Errorf("TopProducts[0] = %+v, want Пицца 5000", b.TopProducts[0])
	}

	// BottomProducts: Вода первая (150, убыточная); Салфетка отброшена (0 продаж)
	if len(b.BottomProducts) != 2 {
		t.Fatalf("BottomProducts: %d, want 2 (Салфетка исключена): %+v", len(b.BottomProducts), b.BottomProducts)
	}
	water := b.BottomProducts[0]
	if water.Name != "Вода" || water.Amount != 150 || water.Quantity != 5 {
		t.Errorf("первый аутсайдер = %+v, want Вода amount=150 qty=5", water)
	}
	if water.Profit == nil || *water.Profit != -15 {
		t.Errorf("Вода.Profit = %v, want -15 (себестоимость в фикстуре есть)", water.Profit)
	}
}

// TestProductsNoProfitData — когда в строках нет profit (живой API из order_items),
// поле Profit остаётся nil и выпадает из снимка: advisor не должен судить о прибыльности.
func TestProductsNoProfitData(t *testing.T) {
	productsNow := dooglys.Result{Rows: []dooglys.Row{
		{"name": "Ролл Миледи", "quantity": 1.0, "amount": 120.0, "discount_sum": 0.0},
		{"name": "Пицца", "quantity": 5.0, "amount": 2500.0, "discount_sum": 50.0},
	}}
	b := BuildInsightBundle(dooglys.Result{Rows: []dooglys.Row{{"sum_all": 2620.0}}},
		dooglys.Result{}, productsNow, "RUB", envelope.Period{}, envelope.Period{})
	for _, p := range append(append([]NamedRow{}, b.TopProducts...), b.BottomProducts...) {
		if p.Profit != nil {
			t.Errorf("%s: Profit=%v, want nil при отсутствии себестоимости", p.Name, *p.Profit)
		}
	}
}

// TestBuildInsightBundle_EmptyPrev — пустой предыдущий период.
func TestBuildInsightBundle_EmptyPrev(t *testing.T) {
	paymentNow := dooglys.Result{Rows: []dooglys.Row{{"sum_all": 1000.0}}}
	b := BuildInsightBundle(paymentNow, dooglys.Result{}, dooglys.Result{}, "RUB",
		envelope.Period{}, envelope.Period{})
	if !b.Revenue.EmptyPrev || b.Revenue.DeltaPct != 0 {
		t.Errorf("при пустом prev ждём EmptyPrev=true и DeltaPct=0, got %+v", b.Revenue)
	}
	if !b.AvgCheck.EmptyPrev {
		t.Errorf("AvgCheck.EmptyPrev должен быть true при пустом prev, got %+v", b.AvgCheck)
	}
}

// TestRelativeLossMetrics — guard'ы относительных метрик: деление на ноль и нулевые потери.
func TestRelativeLossMetrics(t *testing.T) {
	// revenue=0 → доли не считаем (защита от деления на ноль), даже если возвраты/скидки есть.
	zeroRev := BuildInsightBundle(
		dooglys.Result{Rows: []dooglys.Row{{"sum_all": 0.0, "return_sum": 50.0}}},
		dooglys.Result{},
		dooglys.Result{Rows: []dooglys.Row{{"name": "X", "quantity": 1.0, "amount": 0.0, "discount_sum": 20.0}}},
		"RUB", envelope.Period{}, envelope.Period{})
	if zeroRev.ReturnRate != 0 || zeroRev.DiscountShare != 0 {
		t.Errorf("при revenue=0 ждём ReturnRate=0 DiscountShare=0, got %v/%v", zeroRev.ReturnRate, zeroRev.DiscountShare)
	}

	// Возвраты=0 → ReturnRate=0; скидки идут долей от выручки.
	noReturns := BuildInsightBundle(
		dooglys.Result{Rows: []dooglys.Row{{"sum_all": 1000.0, "return_sum": 0.0}}},
		dooglys.Result{},
		dooglys.Result{Rows: []dooglys.Row{{"name": "X", "quantity": 1.0, "amount": 1000.0, "discount_sum": 250.0}}},
		"RUB", envelope.Period{}, envelope.Period{})
	if noReturns.ReturnRate != 0 {
		t.Errorf("при нулевых возвратах ждём ReturnRate=0, got %v", noReturns.ReturnRate)
	}
	if noReturns.DiscountShare != 25 {
		t.Errorf("DiscountShare = %v, want 25 (250/1000)", noReturns.DiscountShare)
	}
}

// TestAvgCheckWeighted — взвешенный средний чек корректнее наивного.
func TestAvgCheckWeighted(t *testing.T) {
	// Два дня: 1000/1 и 100/10 → наивное среднее ≠ взвешенное
	// Взвешенное: (1000+100)/(1+10) = 1100/11 = 100
	rows := []dooglys.Row{
		{"sum_all": 1000.0, "kol_vo_chekov": 1.0},
		{"sum_all": 100.0, "kol_vo_chekov": 10.0},
	}
	got := round2(avgCheckOf(rows))
	if got != 100 {
		t.Errorf("avgCheckOf = %v, want 100", got)
	}
}

// TestChannelMix — нулевые каналы отброшены, сортировка по убыванию суммы.
func TestChannelMix(t *testing.T) {
	rows := []dooglys.Row{
		{"sum_card": 300.0, "sum_cash": 100.0, "onlayn": 0.0, "sbp": 0.0},
	}
	mix := channelMix(rows)
	if len(mix) != 2 {
		t.Fatalf("channelMix len=%d, want 2", len(mix))
	}
	if mix[0].Label != "Карта" || mix[0].Share != 75 {
		t.Errorf("mix[0] = %+v, want Карта 75%%", mix[0])
	}
	if mix[1].Label != "Наличные" || mix[1].Share != 25 {
		t.Errorf("mix[1] = %+v, want Наличные 25%%", mix[1])
	}
}

// TestChannelMix_NegativeChannelDropped — канал, ушедший в минус из-за возвратов,
// не показываем и не даём ему испортить доли. Регрессия: онлайн-возврат без
// онлайн-продаж раньше давал «карта 111%, онлайн -19%».
func TestChannelMix_NegativeChannelDropped(t *testing.T) {
	// Карта 2903 (без возвратов), наличные 205 (нетто после возврата),
	// онлайн -500 (только возврат, продаж нет) — как в боевом мае.
	rows := []dooglys.Row{
		{"sum_card": 2903.0, "sum_cash": 205.0, "onlayn": -500.0, "sbp": 0.0},
	}
	mix := channelMix(rows)
	if len(mix) != 2 {
		t.Fatalf("channelMix len=%d, want 2 (онлайн в минусе отброшен): %+v", len(mix), mix)
	}
	var sum float64
	for _, c := range mix {
		if c.Share < 0 || c.Share > 100 {
			t.Errorf("доля канала вне [0,100]: %+v", c)
		}
		if c.Label == "Онлайн" {
			t.Errorf("онлайн с отрицательным притоком не должен попадать в расклад: %+v", c)
		}
		sum += c.Share
	}
	// Доли нормированы по положительной базе → в сумме 100% (±округление).
	if sum < 99.9 || sum > 100.1 {
		t.Errorf("сумма долей = %v, want ≈100", sum)
	}
}
