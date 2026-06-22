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

// ascRe/descRe — направление рейтинга по смыслу запроса. Модель часто забывает
// выставить order на top_n («что не покупают» давало desc → показывало ЛУЧШИЕ).
// order не проверяется бенчмарком, поэтому фиксим детерминированно, без риска.
var ascRe = regexp.MustCompile(`не\s+покупа|не\s+бер[уёе]т|не\s+прода[ёе]тся|хуж|худш|меньше\s+всего|реже\s+всего|неходов|непопуляр|аутсайдер|антирейтинг|маленьк\w*\s+спрос|низк\w*\s+спрос|минимальн|наименьш`)
var descRe = regexp.MustCompile(`лучш|^топ|\sтоп|больше\s+всего|популярн|сам\w+\s+продава|хит\s+прода|ходов|доходн|прибыльн|чаще\s+всего|наибольш`)

// Refine — детерминированная пост-обработка плана (после планировщика): надёжно
// доводит то, что модель делает нестабильно. Применяется в app и eval (зеркало прода).
func Refine(query string, p *plan.AnalysisPlan) {
	RefineProductContribution(query, p)
	RefineTopNOrder(query, p)
}

// RefineTopNOrder выставляет направление рейтинга (asc — худшие, desc — лучшие)
// по смыслу запроса, если он однозначен. Только для top_n.
func RefineTopNOrder(query string, p *plan.AnalysisPlan) {
	if p.Method != "top_n" {
		return
	}
	q := strings.ToLower(query)
	asc, desc := ascRe.MatchString(q), descRe.MatchString(q)
	switch {
	case asc && !desc:
		p.Order = "asc"
	case desc && !asc:
		p.Order = "desc"
	}
}

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
	// Приводим поля к валидным для products: иначе оставшиеся от модели метрики
	// payment (sum_all) не пройдут валидацию и ответ выродится в «не умею».
	p.Metrics = []string{"amount"}
	p.GroupBy = []string{"name"}
	if p.CompareTo == nil {
		p.CompareTo = &plan.Period{Kind: "relative", Token: "prev_period"}
	}
}
