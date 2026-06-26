package app

import (
	"strings"

	"dgsbot/internal/catalog"
	"dgsbot/internal/envelope"
	"dgsbot/internal/plan"
)

// emptyResultMessage — честный ответ, когда данных для отчёта нет.
func emptyResultMessage(e envelope.Envelope) string {
	return "За период " + e.Period.From + " … " + e.Period.To +
		" данных для отчёта нет. Попробуйте другой период или уточните запрос."
}

// replyForIntent формирует текстовый ответ для не-отчётных интентов.
func (a *App) replyForIntent(p plan.AnalysisPlan) string {
	switch p.EffectiveIntent() {
	case "help":
		return a.helpText()
	case "smalltalk":
		if p.Reply != "" {
			return p.Reply
		}
		return "Здравствуйте! Я помогаю с аналитикой вашего заведения. " + a.helpHint()
	case "off_topic":
		// Детерминированные пост-правила (напр. рейтинг по сотрудникам) кладут готовый
		// честный текст в Reply — отдаём его. Иначе общий отказ вне компетенции.
		if p.Reply != "" {
			return p.Reply
		}
		return "Я отвечаю на вопросы по аналитике вашего заведения. " + a.helpHint()
	default:
		return a.helpText()
	}
}

// filterLabels — человеческие имена фильтров для честного сообщения о недоступном разрезе.
var filterLabels = map[string]string{
	"sale_point":       "точка",
	"locality":         "город",
	"product_category": "категория",
	"product":          "товар",
	"user":             "сотрудник",
	"payment_type":     "тип оплаты",
	"source":           "источник",
}

// skippedFilterMessage — честный ответ, когда запрошенный разрез отчёт не поддерживает
// (фильтр построен, но в отчёте нет такой колонки и он был отброшен). Лучше прямо сказать,
// чем показать полный отчёт как будто это ответ на отфильтрованный запрос.
func (a *App) skippedFilterMessage(rep catalog.Report, skipped []string) string {
	labels := make([]string, 0, len(skipped))
	for _, f := range skipped {
		if l, ok := filterLabels[f]; ok {
			labels = append(labels, l)
		} else {
			labels = append(labels, f)
		}
	}
	return "Отчёт «" + rep.Name + "» не поддерживает разрез по: " + strings.Join(labels, ", ") +
		". Уберите этот фильтр или выберите другой отчёт. " + a.helpHint()
}

// outOfScopeMessage — честный ответ, когда запрос вышел за white-list
// (запрошены поля/разбивки/фильтры, которых нет в каталоге). Не строим неверный
// отчёт и не молчим — объясняем границы и подсказываем, что доступно.
func (a *App) outOfScopeMessage() string {
	return "Такой разрез я пока не умею собирать — доступны только отчёты из каталога " +
		"и их стандартные разбивки. " + a.helpHint()
}

// helpText — список возможностей, собранный из каталога (не выдумывается моделью).
func (a *App) helpText() string {
	var names []string
	for _, slug := range a.cat.Slugs() {
		if r, ok := a.cat.Report(slug); ok {
			names = append(names, r.Name)
		}
	}
	return "Я — ассистент по аналитике вашего заведения. Могу показать отчёты: " +
		strings.Join(names, ", ") + ".\n" +
		"Также объясняю динамику — например «почему упала выручка за месяц».\n" +
		a.helpHint()
}

func (a *App) helpHint() string {
	return "Спросите, например: «выручка за неделю», «топ товаров за месяц», «сравни выручку с прошлой неделей»."
}
