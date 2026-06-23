package engine

import (
	"sort"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

const bottomProductsN = 5
const topProductsN = 5

// Trend — показатель за период с дельтой к предыдущему. Числа считает Go (источник истины).
// EmptyPrev=true → предыдущий период пуст, проценты не считаем (как empty_prev в contribution).
type Trend struct {
	Now       float64 `json:"now"`
	Prev      float64 `json:"prev"`
	DeltaAbs  float64 `json:"delta_abs"`
	DeltaPct  float64 `json:"delta_pct"`
	EmptyPrev bool    `json:"empty_prev"`
}

// Component — вклад одного канала/категории в итог (для ChannelMix).
type Component struct {
	Label string  `json:"label"`
	Now   float64 `json:"now"`
	Share float64 `json:"share"` // % от суммарной выручки текущего периода
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
type InsightBundle struct {
	Period     envelope.Period `json:"period"`
	PrevPeriod envelope.Period `json:"prev_period"`
	Currency   string          `json:"currency"`

	Revenue        Trend       `json:"revenue"`         // sum_all: текущий vs предыдущий
	AvgCheck       Trend       `json:"avg_check"`       // взвешенный средний чек (sum_all / kol_vo_chekov)
	ReturnsSum     Trend       `json:"returns_sum"`     // деньги, ушедшие в возвраты
	ReturnCount    float64     `json:"return_count"`    // число возвратов за текущий период
	Discounts      float64     `json:"discounts"`       // сумма выданных скидок (products.discount_sum)
	ChannelMix     []Component `json:"channel_mix"`     // каналы оплаты по убыванию суммы (только ненулевые)
	TopProducts    []NamedRow  `json:"top_products"`    // топ позиций по выручке, N=5
	BottomProducts []NamedRow  `json:"bottom_products"` // аутсайдеры по выручке (ненулевые продажи), N=5
}

// channelDefs — каналы оплаты: поле в payment-отчёте → человекочитаемое название.
var channelDefs = []struct{ key, label string }{
	{"sum_card", "Карта"},
	{"sum_cash", "Наличные"},
	{"onlayn", "Онлайн"},
	{"sbp", "СБП"},
}

// BuildInsightBundle собирает снимок из УЖЕ выбранных результатов (payment текущий/предыдущий,
// products текущий). Чистая функция над данными — полностью детерминирована и тестируема.
func BuildInsightBundle(paymentNow, paymentPrev, productsNow dooglys.Result,
	currency string, period, periodPrev envelope.Period) InsightBundle {

	revenueNow := sumField(paymentNow.Rows, "sum_all")
	revenuePrev := sumField(paymentPrev.Rows, "sum_all")

	return InsightBundle{
		Period:         period,
		PrevPeriod:     periodPrev,
		Currency:       currency,
		Revenue:        trendOf(revenueNow, revenuePrev),
		AvgCheck:       trendOf(avgCheckOf(paymentNow.Rows), avgCheckOf(paymentPrev.Rows)),
		ReturnsSum:     trendOf(sumField(paymentNow.Rows, "return_sum"), sumField(paymentPrev.Rows, "return_sum")),
		ReturnCount:    round2(sumField(paymentNow.Rows, "return_count")),
		Discounts:      round2(sumField(productsNow.Rows, "discount_sum")),
		ChannelMix:     channelMix(paymentNow.Rows, revenueNow),
		TopProducts:    productsByAmount(productsNow.Rows, topProductsN, false),
		BottomProducts: productsByAmount(productsNow.Rows, bottomProductsN, true),
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

// avgCheckOf вычисляет взвешенный средний чек: sum(sum_all) / sum(kol_vo_chekov).
// Наивное среднее «средних по строкам» искажает при разных объёмах дней.
func avgCheckOf(rows []dooglys.Row) float64 {
	totalAmount := sumField(rows, "sum_all")
	totalReceipts := sumField(rows, "kol_vo_chekov")
	if totalReceipts == 0 {
		return 0
	}
	return totalAmount / totalReceipts
}

// channelMix возвращает ненулевые каналы оплаты, отсортированные по сумме убывания.
// Share — доля канала в общей выручке текущего периода (%).
func channelMix(rows []dooglys.Row, total float64) []Component {
	out := make([]Component, 0, len(channelDefs))
	for _, ch := range channelDefs {
		now := sumField(rows, ch.key)
		if now == 0 {
			continue
		}
		out = append(out, Component{
			Label: ch.label,
			Now:   round2(now),
			Share: shareOf(now, total),
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Now > out[j].Now })
	return out
}

// productsByAmount агрегирует строки по имени, фильтрует нулевые продажи и возвращает N
// позиций: аутсайдеры (asc=true) или лидеры (asc=false) по выручке.
func productsByAmount(rows []dooglys.Row, n int, asc bool) []NamedRow {
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
			continue
		}
		nr.Quantity = round2(nr.Quantity)
		nr.Amount = round2(nr.Amount)
		nr.Profit = round2(nr.Profit)
		nr.Discount = round2(nr.Discount)
		out = append(out, *nr)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if asc {
			return out[i].Amount < out[j].Amount
		}
		return out[i].Amount > out[j].Amount
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
