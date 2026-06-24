// Package telegram — Telegram-транспорт: тонкий адаптер поверх app.Ask.
// Бизнес-логики нет — только приём апдейта, вызов Ask и рендер Answer.
package telegram

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"dgsbot/internal/app"
	"dgsbot/internal/envelope"
	"dgsbot/internal/export"
)

// maxInlineRows — максимум строк таблицы для показа inline-блоком в чате.
// Больше → сводка + xlsx-файл.
const maxInlineRows = 8

// maxInlineCols — максимум колонок для inline-блока (мобильная ширина ~33 символа).
const maxInlineCols = 3

// Document — xlsx-файл, готовый к отправке в Telegram.
type Document struct {
	Name string
	Data []byte
}

// Render конвертирует Answer в текст сообщения и опциональный документ.
// Чистая функция — вся логика тестируется без сети и Telegram API.
//
// Правило (из docs/08-telegram-transport.md §4):
//  1. Всегда отдаём Answer.Text.
//  2. Envelope пустой → только текст.
//  3. Маленький (≤ maxInlineRows строк, ≤ maxInlineCols колонок) → текст + моноширинный блок.
//  4. Крупный/широкий → текст + сводка (итого/макс/мин) + xlsx-документ.
func Render(ans app.Answer) (text string, doc *Document) {
	e := ans.Envelope
	if e == nil || len(e.Rows) == 0 {
		return ans.Text, nil
	}

	if isInline(e) {
		return ans.Text + "\n\n" + inlineTable(e), nil
	}

	// Крупный/широкий — сводка в тексте + xlsx-файл.
	summary := summaryBlock(e)
	if summary != "" {
		text = ans.Text + "\n\n" + summary
	} else {
		text = ans.Text
	}

	data, err := export.XLSX(e)
	if err != nil {
		// Не удалось собрать xlsx — отдаём хотя бы текст со сводкой.
		return text, nil
	}
	return text, &Document{Name: export.Filename(e), Data: data}
}

// isInline сообщает, помещается ли таблица в моноширинный блок прямо в чат.
func isInline(e *envelope.Envelope) bool {
	return len(e.Rows) <= maxInlineRows && len(e.Columns) <= maxInlineCols
}

// inlineTable строит компактную моноширинную таблицу для inline-вывода в Telegram.
// Формат: выровненные колонки, влезает в мобильную ширину (~33 символа).
func inlineTable(e *envelope.Envelope) string {
	// Берём только первые maxInlineCols колонок.
	cols := e.Columns
	if len(cols) > maxInlineCols {
		cols = cols[:maxInlineCols]
	}

	// Вычисляем ширину каждой колонки по максимуму заголовка и значений.
	widths := make([]int, len(cols))
	for i, col := range cols {
		widths[i] = utf8.RuneCountInString(col.Label)
	}
	for _, row := range e.Rows {
		for i, col := range cols {
			v := fmt.Sprintf("%v", row[col.Key])
			if n := utf8.RuneCountInString(v); n > widths[i] {
				widths[i] = n
			}
		}
	}

	var buf bytes.Buffer

	// Заголовок.
	for i, col := range cols {
		if i > 0 {
			buf.WriteString("  ")
		}
		buf.WriteString(padRight(col.Label, widths[i]))
	}
	buf.WriteByte('\n')

	// Разделитель.
	for i, w := range widths {
		if i > 0 {
			buf.WriteString("  ")
		}
		buf.WriteString(strings.Repeat("-", w))
	}
	buf.WriteByte('\n')

	// Строки.
	for _, row := range e.Rows {
		for i, col := range cols {
			if i > 0 {
				buf.WriteString("  ")
			}
			v := fmt.Sprintf("%v", row[col.Key])
			buf.WriteString(padRight(v, widths[i]))
		}
		buf.WriteByte('\n')
	}

	return buf.String()
}

// summaryBlock строит текстовую сводку (итого/макс/мин) из envelope.Summary и Rows.
// Используется вместо полной таблицы при крупных/широких отчётах.
func summaryBlock(e *envelope.Envelope) string {
	var parts []string

	// Итого из Summary.
	if total, ok := summaryTotal(e); ok {
		parts = append(parts, "Итого: "+total)
	}

	// Максимум и минимум по первой числовой колонке из Rows.
	if maxRow, minRow, col, ok := maxMinRows(e); ok {
		dateKey := dateColumn(e)
		if dateKey != "" {
			parts = append(parts,
				fmt.Sprintf("Макс: %v — %v", maxRow[dateKey], maxRow[col]),
				fmt.Sprintf("Мин: %v — %v", minRow[dateKey], minRow[col]),
			)
		}
	}

	return strings.Join(parts, " · ")
}

// summaryTotal возвращает строку суммарного итога из первой числовой колонки Summary.
func summaryTotal(e *envelope.Envelope) (string, bool) {
	for _, col := range e.Columns {
		if v, ok := e.Summary[col.Key]; ok && v != 0 {
			return fmt.Sprintf("%.0f %s", v, e.Currency), true
		}
	}
	return "", false
}

// maxMinRows находит строки с максимальным и минимальным значением первой числовой колонки.
func maxMinRows(e *envelope.Envelope) (maxRow, minRow map[string]any, colKey string, ok bool) {
	// Ищем первую числовую (не date/string) колонку.
	for _, col := range e.Columns {
		if col.Unit == "date" || col.Unit == "string" || col.Unit == "" {
			continue
		}
		colKey = col.Key
		break
	}
	if colKey == "" || len(e.Rows) == 0 {
		return nil, nil, "", false
	}

	maxRow, minRow = e.Rows[0], e.Rows[0]
	maxVal, minVal := toF(e.Rows[0][colKey]), toF(e.Rows[0][colKey])
	for _, row := range e.Rows[1:] {
		v := toF(row[colKey])
		if v > maxVal {
			maxVal, maxRow = v, row
		}
		if v < minVal {
			minVal, minRow = v, row
		}
	}
	return maxRow, minRow, colKey, true
}

// dateColumn возвращает ключ первой колонки типа "date" (для сводки макс/мин).
func dateColumn(e *envelope.Envelope) string {
	for _, col := range e.Columns {
		if col.Unit == "date" {
			return col.Key
		}
	}
	return ""
}

func toF(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

func padRight(s string, width int) string {
	n := utf8.RuneCountInString(s)
	if n >= width {
		return s
	}
	return s + strings.Repeat(" ", width-n)
}
