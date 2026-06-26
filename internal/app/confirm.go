package app

import (
	"strings"

	"dgsbot/internal/catalog"
	"dgsbot/internal/plan"
)

// confirmPrompt — эхо интерпретации плана для подтверждения (plan-confirm).
// Показываем, КАК мы поняли запрос, и просим подтвердить, прежде чем исполнять.
func confirmPrompt(p plan.AnalysisPlan, c *catalog.Catalog) string {
	return "Правильно понимаю: " + describePlan(p, c) +
		"? Ответьте «да» — выполню, или уточните запрос."
}

// describePlan собирает человекочитаемую интерпретацию плана (отчёт + период +
// разрезы) для эха подтверждения. Числа не считает — только переводит план в слова.
func describePlan(p plan.AnalysisPlan, c *catalog.Catalog) string {
	name := p.Report
	if r, ok := c.Report(p.Report); ok {
		name = r.Name
	}
	var b strings.Builder
	switch p.Method {
	case "top_n":
		if p.Order == "asc" {
			b.WriteString("антирейтинг по отчёту «" + name + "»")
		} else {
			b.WriteString("топ по отчёту «" + name + "»")
		}
	case "compare":
		b.WriteString("сравнение периодов по отчёту «" + name + "»")
	case "contribution":
		b.WriteString("разбор вклада по отчёту «" + name + "»")
	default:
		b.WriteString("отчёт «" + name + "»")
	}
	if ph := periodPhrase(p.Period); ph != "" {
		b.WriteString(" за " + ph)
	}
	for _, f := range p.Filters {
		if len(f.Values) == 0 {
			continue
		}
		label := filterLabels[f.Field]
		if label == "" {
			label = f.Field
		}
		b.WriteString(", " + label + ": " + strings.Join(f.Values, ", "))
	}
	return b.String()
}

// periodTokenPhrases — человеческие названия токенов периода для эха подтверждения.
var periodTokenPhrases = map[string]string{
	"today":         "сегодня",
	"yesterday":     "вчера",
	"last_7_days":   "последние 7 дней",
	"last_14_days":  "последние 14 дней",
	"last_30_days":  "последние 30 дней",
	"last_90_days":  "последние 90 дней",
	"last_3_months": "последние 3 месяца",
	"this_week":     "эту неделю",
	"last_week":     "прошлую неделю",
	"this_month":    "этот месяц",
	"last_month":    "прошлый месяц",
}

// periodPhrase переводит период плана в человеческую фразу для эха.
func periodPhrase(p plan.Period) string {
	if p.Kind == "explicit" {
		if p.From != "" && p.To != "" {
			return "период с " + p.From + " по " + p.To
		}
		return ""
	}
	if ph, ok := periodTokenPhrases[p.Token]; ok {
		return ph
	}
	return p.Token // неизвестный токен — показываем как есть, чем молчим
}

// isAffirmation распознаёт короткое подтверждение («да», «верно», «ага», «ок»…)
// в ответ на эхо плана. Намеренно консервативна: подтверждением считается только
// реплика, целиком состоящая из утвердительных слов (≤3). «да, покажи товары» НЕ
// подтверждает устаревший план — такая реплика перепланируется заново.
func isAffirmation(text string) bool {
	t := strings.ToLower(text)
	t = strings.Map(func(r rune) rune {
		switch r {
		case ',', '.', '!', ';', '…':
			return ' '
		}
		return r
	}, t)
	fields := strings.Fields(t)
	if len(fields) == 0 || len(fields) > 3 {
		return false
	}
	for _, f := range fields {
		if !affirmationWords[f] {
			return false
		}
	}
	return true
}

// affirmationWords — закрытый набор утвердительных слов (config-as-data, не фаззи).
var affirmationWords = map[string]bool{
	"да": true, "ага": true, "угу": true, "верно": true, "точно": true,
	"именно": true, "правильно": true, "подтверждаю": true, "согласен": true,
	"давай": true, "поехали": true, "погнали": true, "ок": true, "окей": true,
	"всё": true, "все": true, "так": true,
	"yes": true, "yep": true, "yeah": true, "ok": true, "okay": true, "sure": true,
}
