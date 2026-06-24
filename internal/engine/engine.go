// Package engine — детерминированные вычисления над данными отчётов.
// На M1 реализован метод plain (проекция метрик + итоги). compare/contribution — M3.
package engine

import (
	"sort"
	"strings"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
	"dgsbot/internal/plan"
)

// Plain строит envelope из выборки: колонки = group_by + metrics, итоги = суммы числовых метрик.
// Строки агрегируются по измерению (group_by), числа считаются здесь; LLM их не трогает.
func Plain(p plan.AnalysisPlan, rep catalog.Report, res dooglys.Result,
	tenantID, currency string, period envelope.Period) envelope.Envelope {

	cols := buildColumns(p, rep, res)
	rows := buildRows(cols, p, rep, res)
	summary := buildSummary(cols, p, rep, res)

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

// TopN строит отсортированный по метрике рейтинг строк и берёт первые N.
// Order "asc" → худшие (по возрастанию), иначе → лучшие (по убыванию).
// Итоги считаются по всей выборке, а не по обрезанному топу.
func TopN(p plan.AnalysisPlan, rep catalog.Report, res dooglys.Result,
	tenantID, currency string, period envelope.Period) envelope.Envelope {

	cols := buildColumns(p, rep, res)
	rows := buildRows(cols, p, rep, res)
	summary := buildSummary(cols, p, rep, res)

	sortKey := sortKeyFor(p, cols, rep)
	if sortKey != "" {
		asc := p.Order == "asc"
		sort.SliceStable(rows, func(i, j int) bool {
			a, _ := toFloat(rows[i][sortKey])
			b, _ := toFloat(rows[j][sortKey])
			if asc {
				return a < b
			}
			return a > b
		})
	}

	limit := p.TopN
	if limit <= 0 {
		limit = 10
	}
	if len(rows) > limit {
		rows = rows[:limit]
	}

	return envelope.Envelope{
		Type:     res.Report,
		TenantID: tenantID,
		Period:   period,
		Currency: currency,
		Columns:  cols,
		Rows:     rows,
		Summary:  summary,
		Meta: map[string]any{
			"method": "top_n", "row_count": len(rows),
			"sort_by": sortKey, "order": orderOr(p.Order),
		},
	}
}

// buildColumns: сначала измерения, затем метрики (без дублей и без PII).
// Метрику, которой источник не отдаёт НИ в одной строке, выбрасываем: иначе колонка
// рисуется нулями и вводит в заблуждение (напр. «Ожидаемая прибыль 0,00 ₽» у живого
// товарного отчёта из order_items, где себестоимости нет). Размерности (group_by) и
// вычисляемый sredniy_chek (его считает buildRows, в сырых строках его может не быть)
// не трогаем. При пустой выборке фильтр не применяем — это ветка «данных нет».
func buildColumns(p plan.AnalysisPlan, rep catalog.Report, res dooglys.Result) []envelope.Column {
	var cols []envelope.Column
	seen := map[string]bool{}
	dimKey := map[string]bool{}
	for _, g := range p.GroupBy {
		dimKey[g] = true
	}
	addCol := func(key string) {
		if seen[key] {
			return
		}
		f, ok := rep.FieldByKey(key)
		if !ok || f.PII {
			return
		}
		// Метрика отсутствует во всех строках источника → колонка-пустышка, пропускаем.
		if len(res.Rows) > 0 && !dimKey[key] && key != "sredniy_chek" && !keyInAnyRow(res.Rows, key) {
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
	return cols
}

// keyInAnyRow сообщает, присутствует ли ключ хотя бы в одной строке выборки (наличие
// поля, а не ненулевое значение): отличает «источник не отдаёт метрику» от «метрика=0».
func keyInAnyRow(rows []dooglys.Row, key string) bool {
	for _, r := range rows {
		if _, ok := r[key]; ok {
			return true
		}
	}
	return false
}

// buildRows проецирует строки на колонки и агрегирует их по измерению (group_by):
// суммируемые метрики складываются, иначе берётся первое значение. Без group_by — проекция.
// Это схлопывает дубли (напр. один товар несколькими строками в выгрузке Dooglys).
func buildRows(cols []envelope.Column, p plan.AnalysisPlan, rep catalog.Report, res dooglys.Result) []map[string]any {
	dims := dimKeys(cols, p)
	if len(dims) == 0 {
		rows := make([]map[string]any, 0, len(res.Rows))
		for _, r := range res.Rows {
			out := make(map[string]any, len(cols))
			for _, c := range cols {
				out[c.Key] = r[c.Key]
			}
			rows = append(rows, out)
		}
		return dropZeroRows(rows, cols, dims)
	}

	order := make([]string, 0) // ключи групп в порядке появления
	groups := make(map[string]map[string]any)
	for _, r := range res.Rows {
		key := groupKey(r, dims)
		agg, ok := groups[key]
		if !ok {
			agg = make(map[string]any, len(cols))
			for _, d := range dims {
				agg[d] = r[d]
			}
			groups[key] = agg
			order = append(order, key)
		}
		for _, c := range cols {
			if isDim(c.Key, dims) {
				continue
			}
			f, ok := rep.FieldByKey(c.Key)
			if ok && f.Summable() {
				prev, _ := toFloat(agg[c.Key])
				add, _ := toFloat(r[c.Key])
				agg[c.Key] = round2(prev + add)
			} else if _, set := agg[c.Key]; !set {
				agg[c.Key] = r[c.Key] // несуммируемое — первое значение
			}
		}
	}

	rows := make([]map[string]any, 0, len(order))
	for _, key := range order {
		agg := groups[key]
		// Средний чек на строку = выручка/чеки (а не сумма средних).
		if _, requested := indexOf(cols, "sredniy_chek"); requested {
			if rev, ok := toFloat(agg["sum_all"]); ok {
				if checks, ok := toFloat(agg["kol_vo_chekov"]); ok && checks > 0 {
					agg["sredniy_chek"] = round2(rev / checks)
				}
			}
		}
		rows = append(rows, agg)
	}
	return dropZeroRows(rows, cols, dims)
}

// dropZeroRows убирает строки-пустышки: где все числовые метрики = 0 (день/позиция без
// движения — шум в отчёте). Строка остаётся, если есть хоть одна ненулевая метрика или
// непустое неразмерное значение. Отчёты вовсе без числовых колонок не трогаются.
func dropZeroRows(rows []map[string]any, cols []envelope.Column, dims []string) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		keep, hasNumeric := false, false
		for _, c := range cols {
			if isDim(c.Key, dims) {
				continue
			}
			if f, ok := toFloat(r[c.Key]); ok {
				hasNumeric = true
				if f != 0 {
					keep = true
					break
				}
			} else if s, ok := r[c.Key].(string); ok && strings.TrimSpace(s) != "" {
				keep = true // непустое неразмерное значение — строка содержательна
				break
			}
		}
		if keep || !hasNumeric {
			out = append(out, r)
		}
	}
	return out
}

// buildSummary — итоги по всей выборке (суммируемые поля), с корректным средним чеком.
func buildSummary(cols []envelope.Column, p plan.AnalysisPlan, rep catalog.Report, res dooglys.Result) map[string]float64 {
	dims := dimKeys(cols, p)
	summary := map[string]float64{}
	for _, c := range cols {
		if isDim(c.Key, dims) {
			continue
		}
		f, ok := rep.FieldByKey(c.Key)
		if !ok || !f.Summable() {
			continue
		}
		summary[c.Key] = round2(sumField(res.Rows, c.Key))
	}
	if rev, ok := summary["sum_all"]; ok {
		if checks, ok := summary["kol_vo_chekov"]; ok && checks > 0 {
			if _, requested := indexOf(cols, "sredniy_chek"); requested {
				summary["sredniy_chek"] = round2(rev / checks)
			}
		}
	}
	return summary
}

// sortKeyFor выбирает поле сортировки top_n: явный sort_by → первая метрика → первое суммируемое поле.
func sortKeyFor(p plan.AnalysisPlan, cols []envelope.Column, rep catalog.Report) string {
	dims := dimKeys(cols, p)
	valid := func(key string) bool {
		if key == "" || isDim(key, dims) {
			return false
		}
		_, ok := indexOf(cols, key)
		return ok
	}
	if valid(p.SortBy) {
		return p.SortBy
	}
	// Без явного sort_by предпочитаем денежную метрику (выручку), а не количество:
	// «топ товаров» по смыслу — это рейтинг по деньгам, а не по числу позиций.
	for _, m := range p.Metrics {
		if valid(m) {
			if f, ok := rep.FieldByKey(m); ok && f.Unit == "RUB" {
				return m
			}
		}
	}
	for _, m := range p.Metrics {
		if valid(m) {
			return m
		}
	}
	for _, c := range cols {
		if !isDim(c.Key, dims) {
			if f, ok := rep.FieldByKey(c.Key); ok && f.Summable() {
				return c.Key
			}
		}
	}
	return ""
}

func dimKeys(cols []envelope.Column, p plan.AnalysisPlan) []string {
	var dims []string
	for _, g := range p.GroupBy {
		if _, ok := indexOf(cols, g); ok {
			dims = append(dims, g)
		}
	}
	return dims
}

func isDim(key string, dims []string) bool {
	for _, d := range dims {
		if d == key {
			return true
		}
	}
	return false
}

func groupKey(r dooglys.Row, dims []string) string {
	parts := make([]string, len(dims))
	for i, d := range dims {
		if s, ok := r[d].(string); ok {
			parts[i] = s
		}
	}
	return joinKey(parts)
}

func joinKey(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "\x1f"
		}
		out += p
	}
	return out
}

func orderOr(o string) string {
	if o == "asc" {
		return "asc"
	}
	return "desc"
}

func indexOf(cols []envelope.Column, key string) (int, bool) {
	for i, c := range cols {
		if c.Key == key {
			return i, true
		}
	}
	return -1, false
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
