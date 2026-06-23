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
// Структура: выручка + средний чек → каналы → потери (возвраты/скидки) → топ → аутсайдеры.
func Compose(b engine.InsightBundle) string {
	cur := b.Currency
	var sb strings.Builder

	// 1. Контекст: выручка и средний чек.
	if b.Revenue.EmptyPrev {
		fmt.Fprintf(&sb, "Выручка за период: %s (сравнить не с чем — за предыдущий период данных нет). ",
			render.Money(b.Revenue.Now, cur))
	} else {
		fmt.Fprintf(&sb, "Выручка за период: %s — %s к прошлому периоду (%s). ",
			render.Money(b.Revenue.Now, cur),
			directionWord(b.Revenue.DeltaAbs),
			render.Money(b.Revenue.DeltaAbs, cur))
	}
	if !b.AvgCheck.EmptyPrev && b.AvgCheck.Now > 0 {
		fmt.Fprintf(&sb, "Средний чек: %s (был %s, %s). ",
			render.Money(b.AvgCheck.Now, cur),
			render.Money(b.AvgCheck.Prev, cur),
			formatDeltaPct(b.AvgCheck.DeltaPct))
	} else if b.AvgCheck.Now > 0 {
		fmt.Fprintf(&sb, "Средний чек: %s. ", render.Money(b.AvgCheck.Now, cur))
	}

	// 2. Каналы оплаты (если их больше одного — показываем расклад).
	if len(b.ChannelMix) > 1 {
		parts := make([]string, 0, len(b.ChannelMix))
		for _, ch := range b.ChannelMix {
			parts = append(parts, fmt.Sprintf("%s %s (%.0f%%)", ch.Label, render.Money(ch.Now, cur), ch.Share))
		}
		sb.WriteString("Каналы оплаты: " + strings.Join(parts, ", ") + ". ")
	} else if len(b.ChannelMix) == 1 {
		ch := b.ChannelMix[0]
		fmt.Fprintf(&sb, "Все расчёты через %s. ", ch.Label)
	}

	// 3. Драйверы потерь: возвраты и скидки в деньгах И в долях выручки (относительная тяжесть).
	var losses []string
	if b.ReturnsSum.Now > 0 {
		detail := fmt.Sprintf("%g шт.", b.ReturnCount)
		if b.ReturnRate > 0 {
			detail = fmt.Sprintf("%g%% выручки, %s", b.ReturnRate, detail)
		}
		losses = append(losses, fmt.Sprintf("возвраты — %s (%s)", render.Money(b.ReturnsSum.Now, cur), detail))
	}
	if b.Discounts > 0 {
		s := fmt.Sprintf("скидки — %s", render.Money(b.Discounts, cur))
		if b.DiscountShare > 0 {
			s += fmt.Sprintf(" (%g%% выручки)", b.DiscountShare)
		}
		losses = append(losses, s)
	}
	if len(losses) > 0 {
		sb.WriteString("На чём уходят деньги: " + strings.Join(losses, ", ") + ". ")
	}

	// 4. Лидеры продаж (топ по выручке) — на что опираться.
	if len(b.TopProducts) > 0 {
		parts := make([]string, 0, len(b.TopProducts))
		for _, p := range b.TopProducts {
			parts = append(parts, fmt.Sprintf("%s (%s)", p.Name, render.Money(p.Amount, cur)))
		}
		sb.WriteString("Ключевые позиции: " + strings.Join(parts, ", ") + ". ")
	}

	// 5. Аутсайдеры меню: позиции с низкой выручкой (убыточные помечаем явно).
	if len(b.BottomProducts) > 0 {
		parts := make([]string, 0, len(b.BottomProducts))
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

func formatDeltaPct(pct float64) string {
	switch {
	case pct > 0:
		return fmt.Sprintf("+%.1f%%", pct)
	case pct < 0:
		return fmt.Sprintf("%.1f%%", pct)
	default:
		return "без изменений"
	}
}
