// Package render — превращает envelope в человекочитаемый вывод.
// Числа берутся из envelope (Summary/Rows) — модель их не пишет.
package render

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"dgsbot/internal/envelope"
)

const maxRows = 30

// Text рендерит envelope в текстовую таблицу с итогами.
func Text(e envelope.Envelope) string {
	var b strings.Builder

	title := reportTitle(e.Type)
	fmt.Fprintf(&b, "%s — период %s … %s (%s)\n", title, e.Period.From, e.Period.To, e.Period.TZ)

	if e.Narrative != "" {
		b.WriteString("\n" + e.Narrative + "\n")
	}

	if len(e.Rows) == 0 {
		if e.Narrative == "" {
			b.WriteString("\nЗа выбранный период данных нет.\n")
		}
		return b.String()
	}

	// При пустом предыдущем периоде колонка «Доля изменения» бессмыслена —
	// убираем её, чтобы таблица не противоречила нарративу.
	cols := e.Columns
	if e.Meta["empty_prev"] == true {
		filtered := make([]envelope.Column, 0, len(cols))
		for _, c := range cols {
			if c.Key != "share" {
				filtered = append(filtered, c)
			}
		}
		cols = filtered
	}

	// Заголовки и ширины колонок.
	headers := make([]string, len(cols))
	widths := make([]int, len(cols))
	for i, c := range cols {
		headers[i] = c.Label
		widths[i] = utf8.RuneCountInString(c.Label)
	}

	// Формат ячеек + пересчёт ширин.
	cells := make([][]string, 0, len(e.Rows))
	shown := e.Rows
	if len(shown) > maxRows {
		shown = shown[:maxRows]
	}
	for _, r := range shown {
		row := make([]string, len(cols))
		for i, c := range cols {
			s := formatCell(r[c.Key], c.Unit, e.Currency)
			row[i] = s
			if w := utf8.RuneCountInString(s); w > widths[i] {
				widths[i] = w
			}
		}
		cells = append(cells, row)
	}

	b.WriteByte('\n')
	b.WriteString(line(headers, widths))
	b.WriteString(sep(widths))
	for _, row := range cells {
		b.WriteString(line(row, widths))
	}
	if len(e.Rows) > maxRows {
		fmt.Fprintf(&b, "… ещё %d строк\n", len(e.Rows)-maxRows)
	}

	// Итоги. Печатаем заголовок только если есть что показать: у contribution
	// ключи Summary (value_now/delta…) не совпадают с колонками — иначе была бы
	// пустая строка «Итого:» (сводка такого отчёта уже в нарративе).
	var totals strings.Builder
	for _, c := range cols {
		if v, ok := e.Summary[c.Key]; ok {
			fmt.Fprintf(&totals, "  %s: %s\n", c.Label, formatNumber(v, c.Unit, e.Currency))
		}
	}
	if totals.Len() > 0 {
		b.WriteString("\nИтого:\n")
		b.WriteString(totals.String())
	}
	return b.String()
}

func line(cells []string, widths []int) string {
	parts := make([]string, len(cells))
	for i, s := range cells {
		parts[i] = pad(s, widths[i])
	}
	return strings.Join(parts, "  ") + "\n"
}

func sep(widths []int) string {
	parts := make([]string, len(widths))
	for i, w := range widths {
		parts[i] = strings.Repeat("-", w)
	}
	return strings.Join(parts, "  ") + "\n"
}

func pad(s string, w int) string {
	d := w - utf8.RuneCountInString(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

func formatCell(v any, unit, currency string) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return formatNumber(x, unit, currency)
	default:
		return fmt.Sprintf("%v", x)
	}
}

// Money форматирует сумму в валюте (RU-стиль). Экспортируется для нарратора.
func Money(v float64, currency string) string {
	return groupThousands(v, 2) + " " + currencySymbol(currency)
}

// Pct форматирует процент (RU-стиль).
func Pct(v float64) string { return groupThousands(v, 2) + " %" }

func formatNumber(v float64, unit, currency string) string {
	switch unit {
	case "RUB":
		return groupThousands(v, 2) + " " + currencySymbol(currency)
	case "percent":
		return groupThousands(v, 2) + " %"
	case "count":
		return groupThousands(v, 0)
	default:
		return groupThousands(v, 2)
	}
}

// groupThousands форматирует число в RU-стиле: пробел-тысячи, запятая-десятичные.
func groupThousands(v float64, decimals int) string {
	neg := v < 0
	if neg {
		v = -v
	}
	s := fmt.Sprintf("%.*f", decimals, v)

	intPart, frac := s, ""
	if decimals > 0 {
		if i := strings.IndexByte(s, '.'); i >= 0 {
			intPart, frac = s[:i], s[i+1:]
		}
	}

	var out strings.Builder
	n := len(intPart)
	for i, ch := range intPart {
		if i > 0 && (n-i)%3 == 0 {
			out.WriteByte(' ')
		}
		out.WriteRune(ch)
	}
	res := out.String()
	if frac != "" {
		res += "," + frac
	}
	if neg {
		res = "-" + res
	}
	return res
}

func currencySymbol(code string) string {
	switch code {
	case "RUB":
		return "₽"
	default:
		return code
	}
}

func reportTitle(t string) string {
	t = strings.TrimSuffix(strings.TrimSuffix(t, "_compare"), "_contribution")
	switch t {
	case "payment":
		return "Выручка"
	case "products":
		return "Товары"
	case "paycheck":
		return "Чеки"
	case "orders":
		return "Заказы"
	default:
		return t
	}
}
