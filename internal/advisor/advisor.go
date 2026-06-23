// Package advisor — консультант-надстройка над нарратором: на входе детерминированный
// снимок бизнеса (engine.InsightBundle), на выходе — объяснение слабых мест и 1–3
// рекомендации. Числа считает Go (источник истины), advisor лишь формулирует.
package advisor

import (
	"context"
	"fmt"
	"strings"

	"dgsbot/internal/engine"
	"dgsbot/internal/render"
)

// Advisor — формулирует совет по снимку бизнеса.
type Advisor interface {
	Advise(ctx context.Context, b engine.InsightBundle) (string, error)
}

// Template — детерминированный консультант: собирает совет из чисел снимка без LLM.
// Гарантирует отсутствие галлюцинаций; служит и как fallback для LLM-консультанта.
type Template struct{}

// NewTemplate создаёт детерминированный консультант.
func NewTemplate() *Template { return &Template{} }

func (Template) Advise(_ context.Context, b engine.InsightBundle) (string, error) {
	return Compose(b), nil
}

// Compose — детерминированная сборка совета из снимка (и fallback для LLM-консультанта).
// Структура: контекст выручки → драйверы потерь (возвраты/скидки) → аутсайдеры меню → вывод.
func Compose(b engine.InsightBundle) string {
	cur := b.Currency
	var sb strings.Builder

	// 1. Контекст: куда движется выручка.
	if b.Revenue.EmptyPrev {
		fmt.Fprintf(&sb, "Выручка за период: %s (сравнить не с чем — за предыдущий период данных нет). ",
			render.Money(b.Revenue.Now, cur))
	} else {
		fmt.Fprintf(&sb, "Выручка за период: %s — %s к прошлому периоду (%s). ",
			render.Money(b.Revenue.Now, cur),
			directionWord(b.Revenue.DeltaAbs),
			render.Money(b.Revenue.DeltaAbs, cur))
	}

	// 2. Драйверы потерь: возвраты и скидки в деньгах.
	var losses []string
	if b.ReturnsSum.Now > 0 {
		losses = append(losses, fmt.Sprintf("возвраты — %s (%g шт.)", render.Money(b.ReturnsSum.Now, cur), b.ReturnCount))
	}
	if b.Discounts > 0 {
		losses = append(losses, fmt.Sprintf("скидки — %s", render.Money(b.Discounts, cur)))
	}
	if len(losses) > 0 {
		sb.WriteString("На чём уходят деньги: " + strings.Join(losses, ", ") + ". ")
	}

	// 3. Аутсайдеры меню: позиции с низкой выручкой (убыточные помечаем явно).
	if len(b.BottomProducts) > 0 {
		var parts []string
		for _, p := range b.BottomProducts {
			s := fmt.Sprintf("%s (%s", p.Name, render.Money(p.Amount, cur))
			if p.Profit < 0 {
				s += ", в минусе"
			}
			s += ")"
			parts = append(parts, s)
		}
		sb.WriteString("Слабые позиции меню: " + strings.Join(parts, ", ") + ". ")
		sb.WriteString("Их стоит пересмотреть — поднять цену, убрать или заменить.")
	}

	return strings.TrimSpace(sb.String())
}

func directionWord(delta float64) string {
	switch {
	case delta > 0:
		return "рост"
	case delta < 0:
		return "снижение"
	default:
		return "без изменений"
	}
}
