package plan

import (
	"fmt"
	"strings"

	"dgsbot/internal/catalog"
)

// ValidationResult — итог проверки плана по white-list.
type ValidationResult struct {
	OK            bool
	Errors        []string // фатальные нарушения white-list
	NeedClarify   bool     // не хватает обязательных данных
	ClarifyPrompt string   // что спросить у пользователя
}

func (r ValidationResult) Error() string { return strings.Join(r.Errors, "; ") }

// Validate проверяет план против каталога: отчёт/метрики/фильтры из white-list,
// запрет PII-полей, обязательность периода. Возвращает либо ошибки, либо запрос уточнения.
func Validate(p *AnalysisPlan, c *catalog.Catalog) ValidationResult {
	var res ValidationResult

	rep, ok := c.Report(p.Report)
	if !ok {
		res.Errors = append(res.Errors, fmt.Sprintf("report %q вне white-list", p.Report))
		return res // дальше проверять нечего
	}

	// Метрики — только non-PII поля отчёта.
	for _, m := range p.Metrics {
		if !rep.HasNonPIIField(m) {
			res.Errors = append(res.Errors, fmt.Sprintf("metric %q недоступна (нет в отчёте или PII)", m))
		}
	}
	// group_by — тоже из полей отчёта.
	for _, g := range p.GroupBy {
		if !rep.HasNonPIIField(g) {
			res.Errors = append(res.Errors, fmt.Sprintf("group_by %q недоступно", g))
		}
	}

	// Фильтры — из white-list отчёта, enum-значения проверяются.
	for _, f := range p.Filters {
		cf, ok := rep.FilterByField(f.Field)
		if !ok {
			res.Errors = append(res.Errors, fmt.Sprintf("filter %q вне white-list отчёта", f.Field))
			continue
		}
		if cf.Kind == "enum" {
			for _, v := range f.Values {
				if !contains(cf.Enum, v) {
					res.Errors = append(res.Errors, fmt.Sprintf("filter %q: значение %q недопустимо", f.Field, v))
				}
			}
		}
	}

	// Метод по умолчанию.
	if p.Method == "" {
		p.Method = "plain"
	}
	if !contains([]string{"plain", "compare", "contribution", "top_n", "channel_share", "forecast"}, p.Method) {
		res.Errors = append(res.Errors, fmt.Sprintf("method %q неизвестен", p.Method))
	}

	if len(res.Errors) > 0 {
		return res
	}

	// Период обязателен. Если его нет — это не ошибка, а повод уточнить.
	if !hasPeriod(p.Period) {
		res.NeedClarify = true
		res.ClarifyPrompt = "За какой период подготовить отчёт? Например: сегодня, вчера, последние 7 дней, текущий месяц."
		return res
	}

	res.OK = true
	return res
}

func hasPeriod(p Period) bool {
	switch p.Kind {
	case "relative":
		return p.Token != ""
	case "explicit":
		return p.From != "" && p.To != ""
	}
	return false
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
