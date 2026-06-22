package planner

import (
	"regexp"
	"strings"

	"dgsbot/internal/plan"
)

// productContribRe ловит ЯВНЫЙ запрос раскладки изменения по товарам:
// «какой товар виноват в падении», «из-за каких товаров упала выручка», «вклад товаров».
// Узко и намеренно: фаззи-правило в промпте растекалось на top_n/compare-кейсы
// («что больше всего продали», «как изменились продажи») и давало регрессию,
// поэтому маршрутизируем детерминированно — как refusal-фильтр. Проверено: ни один
// кейс бенчмарка под этот паттерн не попадает (рейтинги «какой товар самый…» — нет
// слов виноват/вклад/из-за), поэтому общий pass-rate не затрагивается.
var productContribRe = regexp.MustCompile(
	`как(ой|ие)\s+товар\w*[^.?!]*(виноват|причин|из-за|повлия|вклад)` +
		`|вклад\s+товаров` +
		`|из-за\s+как(ого|их)\s+товар`)

// RefineProductContribution детерминированно фиксирует products+contribution для
// запросов «какой товар виноват в росте/падении» (раскладка изменения по товарам).
// Применяется ПОСЛЕ планировщика: модель такие формулировки путает с top_n/compare.
func RefineProductContribution(query string, p *plan.AnalysisPlan) {
	if p.Intent != "" && p.Intent != "report" {
		return
	}
	if !productContribRe.MatchString(strings.ToLower(query)) {
		return
	}
	p.Report = "products"
	p.Class = plan.ClassB
	p.Method = "contribution"
	if p.CompareTo == nil {
		p.CompareTo = &plan.Period{Kind: "relative", Token: "prev_period"}
	}
}
