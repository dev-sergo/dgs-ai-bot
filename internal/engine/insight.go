package engine

import (
	"sort"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

// bottomProductsN — сколько худших позиций кладём в снимок (кандидаты «убрать из меню»).
const bottomProductsN = 5

// Trend — показатель за период с дельтой к предыдущему. Числа считает Go (источник истины).
// EmptyPrev=true → предыдущий период пуст, проценты не считаем (как empty_prev в contribution).
type Trend struct {
	Now       float64 `json:"now"`
	Prev      float64 `json:"prev"`
	DeltaAbs  float64 `json:"delta_abs"`
	DeltaPct  float64 `json:"delta_pct"`
	EmptyPrev bool    `json:"empty_prev"`
}

// NamedRow — агрегат одной позиции номенклатуры за период.
type NamedRow struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Amount   float64 `json:"amount"`
	Profit   float64 `json:"profit"`
	Discount float64 `json:"discount"`
}

// InsightBundle — детерминированный «снимок бизнеса» за период: набор фактов, поверх
// которых advisor-LLM строит объяснение и рекомендации. Все числа — источник истины.
// Тонкий срез (фокус «на чём теряю / что улучшить»): выручка, возвраты, скидки, аутсайдеры.
type InsightBundle struct {
	Period     envelope.Period `json:"period"`
	PrevPeriod envelope.Period `json:"prev_period"`
	Currency   string          `json:"currency"`

	Revenue        Trend      `json:"revenue"`         // sum_all: текущий vs предыдущий
	ReturnsSum     Trend      `json:"returns_sum"`     // деньги, ушедшие в возвраты
	ReturnCount    float64    `json:"return_count"`    // число возвратов за текущий период
	Discounts      float64    `json:"discounts"`       // сумма выданных скидок (products.discount_sum)
	BottomProducts []NamedRow `json:"bottom_products"` // позиции с наименьшей выручкой (ненулевые продажи)
}

// BuildInsightBundle собирает снимок из УЖЕ выбранных результатов (payment текущий/предыдущий,
// products текущий). Чистая функция над данными — полностью детерминирована и тестируема.
func BuildInsightBundle(paymentNow, paymentPrev, productsNow dooglys.Result,
	currency string, period, periodPrev envelope.Period) InsightBundle {

	return InsightBundle{
		Period:         period,
		PrevPeriod:     periodPrev,
		Currency:       currency,
		Revenue:        trendOf(sumField(paymentNow.Rows, "sum_all"), sumField(paymentPrev.Rows, "sum_all")),
		ReturnsSum:     trendOf(sumField(paymentNow.Rows, "return_sum"), sumField(paymentPrev.Rows, "return_sum")),
		ReturnCount:    round2(sumField(paymentNow.Rows, "return_count")),
		Discounts:      round2(sumField(productsNow.Rows, "discount_sum")),
		BottomProducts: bottomProducts(productsNow.Rows, bottomProductsN),
	}
}

// trendOf собирает Trend из двух сумм с дельтой и процентом (0 при пустой базе).
func trendOf(now, prev float64) Trend {
	return Trend{
		Now:       round2(now),
		Prev:      round2(prev),
		DeltaAbs:  round2(now - prev),
		DeltaPct:  pct(now, prev),
		EmptyPrev: prev == 0,
	}
}

// bottomProducts агрегирует строки по имени и берёт N позиций с наименьшей выручкой
// среди тех, что РЕАЛЬНО продавались (quantity>0) — нулевые продажи не «аутсайдеры»,
// а просто отсутствие движения, советовать по ним нечего.
func bottomProducts(rows []dooglys.Row, n int) []NamedRow {
	agg := map[string]*NamedRow{}
	order := []string{}
	for _, r := range rows {
		name, _ := r["name"].(string)
		if name == "" {
			continue
		}
		nr, ok := agg[name]
		if !ok {
			nr = &NamedRow{Name: name}
			agg[name] = nr
			order = append(order, name)
		}
		q, _ := toFloat(r["quantity"])
		a, _ := toFloat(r["amount"])
		p, _ := toFloat(r["profit"])
		d, _ := toFloat(r["discount_sum"])
		nr.Quantity += q
		nr.Amount += a
		nr.Profit += p
		nr.Discount += d
	}

	out := make([]NamedRow, 0, len(order))
	for _, name := range order {
		nr := agg[name]
		if nr.Quantity <= 0 {
			continue // не продавалось — не аутсайдер
		}
		nr.Quantity = round2(nr.Quantity)
		nr.Amount = round2(nr.Amount)
		nr.Profit = round2(nr.Profit)
		nr.Discount = round2(nr.Discount)
		out = append(out, *nr)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Amount < out[j].Amount })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
