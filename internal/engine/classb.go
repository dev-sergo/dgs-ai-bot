package engine

import (
	"sort"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
	"dgsbot/internal/plan"
)

// components — раскладка метрики на ФИКСИРОВАННЫЕ колонки (для contribution).
// Для payment выручка sum_all == сумме каналов оплаты, поэтому раскладка точная.
var components = map[string][]string{
	"payment": {"sum_card", "sum_cash", "onlayn", "sbp"},
}

// contribByDim — раскладка изменения по ИЗМЕРЕНИЮ (строкам отчёта): slug → {измерение, метрика}.
// Например, «какой товар виноват в падении выручки» = вклад по товарам (name) через выручку (amount).
var contribByDim = map[string]struct{ dim, metric string }{
	"products": {dim: "name", metric: "amount"},
}

// SupportsContribution сообщает, можно ли разложить изменение метрики на компоненты —
// по фиксированным колонкам (payment) ИЛИ по измерению (products по товарам).
// Для отчётов без раскладки contribution вырождается — вызывающий код понижает до compare.
func SupportsContribution(slug string) bool {
	return len(components[slug]) > 0 || contribByDim[slug].dim != ""
}

// NormalizeMethod приводит method плана к выполнимому виду: contribution осмыслен
// только для отчётов с раскладкой на компоненты (payment). Для прочих отчётов он
// вырождается в пустоту — понижаем до compare (суммарное изменение честнее пустой
// раскладки). Единый инвариант для прода и eval, чтобы бенчмарк мерил то, что
// реально увидит пользователь, а не сырой ответ модели.
func NormalizeMethod(p *plan.AnalysisPlan) {
	if p.Method == "contribution" && !SupportsContribution(p.Report) {
		p.Method = "compare"
	}
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

// Contribution раскладывает изменение метрики между периодами на компоненты —
// по фиксированным колонкам (payment) или по измерению (products по товарам).
func Contribution(rep catalog.Report, metric string, now, prev dooglys.Result, topN int,
	tenantID, currency string, periodNow, periodPrev envelope.Period) envelope.Envelope {

	if comps := components[rep.Slug]; len(comps) > 0 {
		rows := make([]contribRow, 0, len(comps))
		vNow := sumField(now.Rows, metric)
		vPrev := sumField(prev.Rows, metric)
		totalDelta := vNow - vPrev
		for _, key := range comps {
			n := sumField(now.Rows, key)
			p := sumField(prev.Rows, key)
			label := key
			if f, ok := rep.FieldByKey(key); ok {
				label = f.Label
			}
			rows = append(rows, contribRow{key, label, round2(n), round2(p), round2(n - p), round2(shareOf(n-p, totalDelta))})
		}
		return contribEnvelope(rep, metric, "Компонента", rows, vNow, vPrev, topN, tenantID, currency, periodNow, periodPrev)
	}

	if cfg := contribByDim[rep.Slug]; cfg.dim != "" {
		return contribByDimension(rep, cfg.metric, cfg.dim, now, prev, topN, tenantID, currency, periodNow, periodPrev)
	}

	// Раскладки нет — суммарное сравнение (NormalizeMethod сюда дойти не должен).
	return Compare(rep, metric, now, prev, tenantID, currency, periodNow, periodPrev)
}

// contribByDimension раскладывает изменение метрики по значениям измерения (напр. по
// товарам через name+amount): для каждого значения считает now/prev/delta и долю.
func contribByDimension(rep catalog.Report, metric, dim string, now, prev dooglys.Result, topN int,
	tenantID, currency string, periodNow, periodPrev envelope.Period) envelope.Envelope {

	type acc struct{ now, prev float64 }
	byKey := map[string]*acc{}
	var order []string
	accumulate := func(rows []dooglys.Row, isNow bool) {
		for _, r := range rows {
			key, _ := r[dim].(string)
			val, _ := toFloat(r[metric])
			a := byKey[key]
			if a == nil {
				a = &acc{}
				byKey[key] = a
				order = append(order, key)
			}
			if isNow {
				a.now += val
			} else {
				a.prev += val
			}
		}
	}
	accumulate(now.Rows, true)
	accumulate(prev.Rows, false)

	vNow := sumField(now.Rows, metric)
	vPrev := sumField(prev.Rows, metric)
	totalDelta := vNow - vPrev

	rows := make([]contribRow, 0, len(order))
	for _, key := range order {
		a := byKey[key]
		rows = append(rows, contribRow{key, key, round2(a.now), round2(a.prev), round2(a.now - a.prev), round2(shareOf(a.now-a.prev, totalDelta))})
	}

	dimLabel := dim
	if f, ok := rep.FieldByKey(dim); ok {
		dimLabel = f.Label
	}
	// Раскладка по товарам может быть длинной — по умолчанию показываем топ движений.
	if topN <= 0 {
		topN = 10
	}
	return contribEnvelope(rep, metric, dimLabel, rows, vNow, vPrev, topN, tenantID, currency, periodNow, periodPrev)
}

// contribEnvelope собирает envelope раскладки: сортирует по модулю изменения, режет до topN.
func contribEnvelope(rep catalog.Report, metric, firstColLabel string, rows []contribRow,
	vNow, vPrev float64, topN int, tenantID, currency string, periodNow, periodPrev envelope.Period) envelope.Envelope {

	sort.SliceStable(rows, func(i, j int) bool { return abs(rows[i].delta) > abs(rows[j].delta) })
	if topN > 0 && len(rows) > topN {
		rows = rows[:topN]
	}

	cols := []envelope.Column{
		{Key: "component", Label: firstColLabel, Unit: "string"},
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

	meta := map[string]any{
		"method":      "contribution",
		"metric":      metric,
		"period_prev": periodPrev.From + "…" + periodPrev.To,
	}
	// Когда предыдущий период пуст, «Доля изменения» — бессмыслица (нет базы для сравнения).
	// Флаг сигнализирует рендеру скрыть эту колонку, чтобы не противоречить нарративу.
	if vPrev == 0 {
		meta["empty_prev"] = true
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
			"delta_abs":  round2(vNow - vPrev),
			"delta_pct":  pct(vNow, vPrev),
		},
		Meta: meta,
	}
}

// shareOf — доля изменения d в общем изменении total, % (0 при нулевом total).
func shareOf(d, total float64) float64 {
	if total == 0 {
		return 0
	}
	return d / total * 100
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
