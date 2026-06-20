package engine

import (
	"sort"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

// components — на какие слагаемые раскладывается метрика отчёта (для contribution).
// Для payment выручка sum_all == сумме каналов оплаты, поэтому раскладка точная.
var components = map[string][]string{
	"payment": {"sum_card", "sum_cash", "onlayn", "sbp"},
}

// SupportsContribution сообщает, можно ли разложить метрику отчёта на компоненты.
// Для отчётов без раскладки contribution выродится в пустоту — вызывающий код
// должен понизить метод до compare.
func SupportsContribution(slug string) bool {
	return len(components[slug]) > 0
}

// Compare сравнивает суммарную метрику между двумя периодами.
func Compare(rep catalog.Report, metric string, now, prev dooglys.Result,
	tenantID, currency string, periodNow, periodPrev envelope.Period) envelope.Envelope {

	vNow := sumField(now.Rows, metric)
	vPrev := sumField(prev.Rows, metric)
	delta := round2(vNow - vPrev)

	return envelope.Envelope{
		Type:     rep.Slug + "_compare",
		TenantID: tenantID,
		Period:   periodNow,
		Currency: currency,
		Summary: map[string]float64{
			"value_now":  round2(vNow),
			"value_prev": round2(vPrev),
			"delta_abs":  delta,
			"delta_pct":  pct(vNow, vPrev),
		},
		Meta: map[string]any{
			"method":      "compare",
			"metric":      metric,
			"period_prev": periodPrev.From + "…" + periodPrev.To,
		},
	}
}

// ContribRow — вклад одной компоненты в изменение метрики.
type contribRow struct {
	key   string
	label string
	now   float64
	prev  float64
	delta float64
	share float64 // доля в общем изменении, %
}

// Contribution раскладывает изменение метрики по компонентам между периодами.
func Contribution(rep catalog.Report, metric string, now, prev dooglys.Result, topN int,
	tenantID, currency string, periodNow, periodPrev envelope.Period) envelope.Envelope {

	comps := components[rep.Slug]
	vNow := sumField(now.Rows, metric)
	vPrev := sumField(prev.Rows, metric)
	totalDelta := vNow - vPrev

	rows := make([]contribRow, 0, len(comps))
	for _, key := range comps {
		n := sumField(now.Rows, key)
		p := sumField(prev.Rows, key)
		d := n - p
		label := key
		if f, ok := rep.FieldByKey(key); ok {
			label = f.Label
		}
		share := 0.0
		if totalDelta != 0 {
			share = d / totalDelta * 100
		}
		rows = append(rows, contribRow{key, label, round2(n), round2(p), round2(d), round2(share)})
	}

	// Сортировка по модулю изменения (по убыванию).
	sort.SliceStable(rows, func(i, j int) bool { return abs(rows[i].delta) > abs(rows[j].delta) })
	if topN > 0 && len(rows) > topN {
		rows = rows[:topN]
	}

	cols := []envelope.Column{
		{Key: "component", Label: "Компонента", Unit: "string"},
		{Key: "delta", Label: "Изменение", Unit: "RUB"},
		{Key: "now", Label: "Текущий", Unit: "RUB"},
		{Key: "prev", Label: "Предыдущий", Unit: "RUB"},
		{Key: "share", Label: "Доля изменения", Unit: "percent"},
	}
	outRows := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		outRows = append(outRows, map[string]any{
			"component": r.label, "delta": r.delta, "now": r.now, "prev": r.prev, "share": r.share,
		})
	}

	return envelope.Envelope{
		Type:     rep.Slug + "_contribution",
		TenantID: tenantID,
		Period:   periodNow,
		Currency: currency,
		Columns:  cols,
		Rows:     outRows,
		Summary: map[string]float64{
			"value_now":  round2(vNow),
			"value_prev": round2(vPrev),
			"delta_abs":  round2(totalDelta),
			"delta_pct":  pct(vNow, vPrev),
		},
		Meta: map[string]any{
			"method":      "contribution",
			"metric":      metric,
			"period_prev": periodPrev.From + "…" + periodPrev.To,
		},
	}
}

func sumField(rows []dooglys.Row, key string) float64 {
	var s float64
	for _, r := range rows {
		if v, ok := toFloat(r[key]); ok {
			s += v
		}
	}
	return s
}

// pct возвращает относительное изменение в процентах (0 при нулевой базе).
func pct(now, prev float64) float64 {
	if prev == 0 {
		return 0
	}
	return round2((now - prev) / prev * 100)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
