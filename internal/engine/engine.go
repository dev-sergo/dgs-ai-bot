// Package engine — детерминированные вычисления над данными отчётов.
// На M1 реализован метод plain (проекция метрик + итоги). compare/contribution — M3.
package engine

import (
	"dgsbot/internal/catalog"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
	"dgsbot/internal/plan"
)

// Plain строит envelope из выборки: колонки = group_by + metrics, итоги = суммы числовых метрик.
// Все числа считаются здесь; LLM их не трогает.
func Plain(p plan.AnalysisPlan, rep catalog.Report, res dooglys.Result,
	tenantID, currency string, period envelope.Period) envelope.Envelope {

	// Порядок колонок: сначала измерения, затем метрики (без дублей и без PII).
	var cols []envelope.Column
	seen := map[string]bool{}
	addCol := func(key string) {
		if seen[key] {
			return
		}
		f, ok := rep.FieldByKey(key)
		if !ok || f.PII {
			return
		}
		seen[key] = true
		cols = append(cols, envelope.Column{Key: f.Key, Label: f.Label, Unit: f.Unit})
	}
	for _, g := range p.GroupBy {
		addCol(g)
	}
	for _, m := range p.Metrics {
		addCol(m)
	}

	// Строки — проекция на выбранные колонки.
	rows := make([]map[string]any, 0, len(res.Rows))
	for _, r := range res.Rows {
		out := make(map[string]any, len(cols))
		for _, c := range cols {
			out[c.Key] = r[c.Key]
		}
		rows = append(rows, out)
	}

	// Итоги — суммы числовых метрик (не измерений).
	dim := map[string]bool{}
	for _, g := range p.GroupBy {
		dim[g] = true
	}
	summary := map[string]float64{}
	for _, c := range cols {
		if dim[c.Key] || !isNumeric(c.Unit) {
			continue
		}
		var sum float64
		for _, r := range res.Rows {
			if v, ok := toFloat(r[c.Key]); ok {
				sum += v
			}
		}
		summary[c.Key] = round2(sum)
	}

	return envelope.Envelope{
		Type:     res.Report,
		TenantID: tenantID,
		Period:   period,
		Currency: currency,
		Columns:  cols,
		Rows:     rows,
		Summary:  summary,
		Meta:     map[string]any{"method": "plain", "row_count": len(rows)},
	}
}

func isNumeric(unit string) bool {
	return unit == "RUB" || unit == "count" || unit == "percent"
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	}
	return 0, false
}

func round2(v float64) float64 {
	// округление до копеек без math (избегаем лишних импортов): *100 + 0.5
	if v >= 0 {
		return float64(int64(v*100+0.5)) / 100
	}
	return float64(int64(v*100-0.5)) / 100
}
