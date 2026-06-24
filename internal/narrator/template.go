package narrator

import (
	"context"
	"fmt"
	"strings"

	"dgsbot/internal/envelope"
	"dgsbot/internal/render"
)

// Template — детерминированный нарратор: собирает фразу из чисел envelope.
// Гарантирует отсутствие галлюцинаций (числа не выдумываются).
type Template struct{}

// NewTemplate создаёт детерминированный нарратор.
func NewTemplate() *Template { return &Template{} }

func (Template) Narrate(_ context.Context, e envelope.Envelope) (string, error) {
	return Compose(e), nil
}

// Compose — общая сборка текста (используется и как fallback для LLM-нарратора).
func Compose(e envelope.Envelope) string {
	s := e.Summary
	now, hasNow := s["value_now"]
	prev := s["value_prev"]
	dPct := s["delta_pct"]
	dAbs := s["delta_abs"]
	if !hasNow {
		return ""
	}

	metric := capitalize(strings.ToLower(metricName(e.Type)))
	var b strings.Builder

	if prev == 0 {
		// Нулевая база: процент не информативен.
		fmt.Fprintf(&b, "%s за период: %s. За предыдущий период данных нет — сравнение недоступно.",
			metric, render.Money(now, e.Currency))
		return b.String()
	}

	// Поправка ложной посылки: вопрос подразумевал одно направление, числа показывают
	// обратное («почему упала» при выросшей выручке). Нейтральные существительные —
	// без согласования рода с метрикой (выручка/показатель). См. planner.PremiseDirection.
	if pd, _ := e.Meta["premise_dir"].(string); pd == "down" && dAbs > 0 {
		b.WriteString("Уточнение: за период не падение, а рост. ")
	} else if pd == "up" && dAbs < 0 {
		b.WriteString("Уточнение: за период не рост, а снижение. ")
	}

	fmt.Fprintf(&b, "%s за период: %s, за предыдущий: %s — %s на %s (%s).",
		metric,
		render.Money(now, e.Currency),
		render.Money(prev, e.Currency),
		direction(dAbs),
		render.Pct(absf(dPct)),
		signedMoney(dAbs, e.Currency),
	)

	// Для contribution — топ вкладчиков в изменение (без процентов: при спаде они путают; % есть в таблице).
	if len(e.Rows) > 0 && hasContribCols(e.Columns) {
		var parts []string
		limit := 3
		for _, r := range e.Rows {
			if limit == 0 {
				break
			}
			d, _ := r["delta"].(float64)
			if d == 0 {
				continue
			}
			comp, _ := r["component"].(string)
			parts = append(parts, fmt.Sprintf("%s %s", comp, signedMoney(d, e.Currency)))
			limit--
		}
		if len(parts) > 0 {
			b.WriteString(" Основной вклад в изменение: " + strings.Join(parts, ", ") + ".")
		}
	}
	return b.String()
}

func metricName(typ string) string {
	switch {
	case strings.HasPrefix(typ, "payment"):
		return "Выручка"
	case strings.HasPrefix(typ, "products"):
		return "Показатель"
	default:
		return "Показатель"
	}
}

func direction(delta float64) string {
	switch {
	case delta > 0:
		return "вырос"
	case delta < 0:
		return "снизился"
	default:
		return "не изменился"
	}
}

func signedMoney(v float64, currency string) string {
	if v > 0 {
		return "+" + render.Money(v, currency)
	}
	return render.Money(v, currency) // отрицательное уже со знаком
}

func hasContribCols(cols []envelope.Column) bool {
	for _, c := range cols {
		if c.Key == "component" {
			return true
		}
	}
	return false
}

func absf(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return strings.ToUpper(string(r[0])) + string(r[1:])
}
