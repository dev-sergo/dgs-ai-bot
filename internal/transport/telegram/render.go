// Package telegram — Telegram-транспорт: тонкий адаптер поверх app.Ask.
// Бизнес-логики нет — только приём апдейта, вызов Ask и рендер Answer.
package telegram

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf8"

	"dgsbot/internal/app"
	"dgsbot/internal/envelope"
	"dgsbot/internal/export"
	"dgsbot/internal/render"
)

// maxInlineRows — максимум строк отчёта для показа прямо в сообщении.
// Больше → сводка + xlsx-файл.
const maxInlineRows = 8

// maxInlineChars — предохранитель на длину inline-сообщения (лимит Telegram 4096).
// Слишком длинный список карточек деградирует в сводку + xlsx.
const maxInlineChars = 3500

// Document — xlsx-файл, готовый к отправке в Telegram.
type Document struct {
	Name string
	Data []byte
}

// Render конвертирует Answer в текст сообщения (HTML parse_mode) и опциональный документ.
// Чистая функция — вся логика тестируется без сети и Telegram API.
//
// Мобильный формат (docs/08-telegram-transport.md §4): таблицы с выравниванием
// пробелами в Telegram разъезжаются (пропорциональный шрифт, узкий экран),
// поэтому строки рендерятся вертикально — карточками «Подпись: значение».
//
// Правила:
//  1. Envelope пустой → Answer.Text (экранированный).
//  2. Маленький отчёт (≤ maxInlineRows строк) → заголовок + карточки + итоги.
//  3. Крупный → заголовок + итоги + макс/мин + xlsx-документ.
func Render(ans app.Answer) (text string, doc *Document) {
	e := ans.Envelope
	if e == nil || len(e.Rows) == 0 {
		return html.EscapeString(ans.Text), nil
	}

	if len(e.Rows) <= maxInlineRows {
		if msg := inlineMessage(e); utf8.RuneCountInString(msg) <= maxInlineChars {
			return msg, nil
		}
	}

	// Крупный отчёт: сводка в тексте + полные данные в xlsx-файле.
	text = summaryMessage(e)
	data, err := export.XLSX(e)
	if err != nil {
		// Не удалось собрать xlsx — отдаём хотя бы сводку.
		return text, nil
	}
	return text, &Document{Name: export.Filename(e), Data: data}
}

// inlineMessage — компактный отчёт целиком в сообщении: заголовок, нарратив,
// строки-карточки, итоги.
func inlineMessage(e *envelope.Envelope) string {
	var b strings.Builder
	b.WriteString(header(e))
	b.WriteString(rowsBlock(e))
	if t := totalsBlock(e); t != "" {
		b.WriteString("\n" + t)
	}
	return strings.TrimRight(b.String(), "\n")
}

// summaryMessage — сводка крупного отчёта: заголовок, нарратив, итоги, макс/мин.
func summaryMessage(e *envelope.Envelope) string {
	parts := []string{strings.TrimRight(header(e), "\n")}
	if t := totalsBlock(e); t != "" {
		parts = append(parts, strings.TrimRight(t, "\n"))
	}
	if mm := maxMinBlock(e); mm != "" {
		parts = append(parts, strings.TrimRight(mm, "\n"))
	}
	n := len(e.Rows)
	parts = append(parts, fmt.Sprintf("Полный отчёт (%d %s) — в файле ниже.", n, rowsWord(n)))
	return strings.Join(parts, "\n\n")
}

// rowsWord — склонение слова «строка» по числу.
func rowsWord(n int) string {
	m10, m100 := n%10, n%100
	switch {
	case m10 == 1 && m100 != 11:
		return "строка"
	case m10 >= 2 && m10 <= 4 && (m100 < 10 || m100 >= 20):
		return "строки"
	default:
		return "строк"
	}
}

// header — эмодзи + жирный заголовок отчёта, период и нарратив (если есть).
func header(e *envelope.Envelope) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s <b>%s</b>\n", typeEmoji(e.Type), html.EscapeString(render.Title(e.Type)))
	if e.Period.From != "" || e.Period.To != "" {
		fmt.Fprintf(&b, "%s — %s\n", html.EscapeString(e.Period.From), html.EscapeString(e.Period.To))
	}
	b.WriteByte('\n')
	if n := strings.TrimSpace(e.Narrative); n != "" {
		b.WriteString(html.EscapeString(n) + "\n\n")
	}
	if badge := deltaBadge(e); badge != "" {
		b.WriteString(badge + "\n\n")
	}
	return b.String()
}

// deltaBadge — строка направления для сравнений: ▲ рост / ▼ падение по delta_pct.
// Пусто, если в Summary нет дельты (обычные отчёты). Экранирования не требует —
// собирается из безопасных символов и уже отформатированного числа.
func deltaBadge(e *envelope.Envelope) string {
	p, ok := e.Summary["delta_pct"]
	if !ok {
		return ""
	}
	if p == 0 {
		return "≈ без изменений к предыдущему периоду"
	}
	arrow, sign := "▲", "+"
	if p < 0 {
		arrow, sign, p = "▼", "−", -p
	}
	return fmt.Sprintf("%s %s%s к предыдущему периоду", arrow, sign, render.Number(p, "percent", ""))
}

// rowsBlock рендерит строки отчёта вертикально.
// Две колонки → строка-список «значение — жирное значение»; больше → карточка на
// строку: первая колонка жирным как заголовок, остальные «Подпись: значение».
func rowsBlock(e *envelope.Envelope) string {
	cols := render.VisibleColumns(*e)
	if len(cols) == 0 {
		return ""
	}

	var b strings.Builder
	if len(cols) <= 2 {
		// Нумеруем именованные списки (топы/антирейтинги) — ранг сразу виден.
		// По датам нумерация бессмысленна, поэтому только для нечисловой первой колонки.
		numbered := cols[0].Unit == "string"
		for i, row := range e.Rows {
			if numbered {
				fmt.Fprintf(&b, "%d. ", i+1)
			}
			b.WriteString(html.EscapeString(render.CellCompact(row[cols[0].Key], cols[0].Unit, e.Currency)))
			if len(cols) == 2 {
				val := render.CellCompact(row[cols[1].Key], cols[1].Unit, e.Currency)
				b.WriteString(" — <b>" + html.EscapeString(val) + "</b>")
			}
			b.WriteByte('\n')
		}
		return b.String()
	}

	for i, row := range e.Rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		head := cols[0].Label + ": " + render.CellCompact(row[cols[0].Key], cols[0].Unit, e.Currency)
		b.WriteString("<b>" + html.EscapeString(head) + "</b>\n")
		for _, c := range cols[1:] {
			raw := row[c.Key]
			if isZeroNum(raw, c.Unit) {
				continue // нулевые деньги/счётчики в карточке — шум, прячем
			}
			v := render.CellCompact(raw, c.Unit, e.Currency)
			if v == "" {
				continue
			}
			b.WriteString(html.EscapeString(c.Label+": "+v) + "\n")
		}
	}
	return b.String()
}

// isZeroNum сообщает, что значение — нулевое число в денежной/счётной/процентной
// колонке (кандидат на скрытие в карточке).
func isZeroNum(v any, unit string) bool {
	switch unit {
	case "RUB", "count", "percent":
		return toF(v) == 0
	}
	return false
}

// totalsBlock — блок «Итого» из envelope.Summary по видимым колонкам.
// Пустой, если ключи Summary не совпадают с колонками (contribution и пр. —
// их сводка уже в нарративе).
func totalsBlock(e *envelope.Envelope) string {
	var b strings.Builder
	for _, c := range render.VisibleColumns(*e) {
		if v, ok := e.Summary[c.Key]; ok {
			b.WriteString(html.EscapeString(c.Label+": "+render.NumberCompact(v, c.Unit, e.Currency)) + "\n")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return "<b>Итого</b>\n" + b.String()
}

// maxMinBlock — строки максимума и минимума по первой числовой колонке (для
// сводки крупного отчёта, когда есть колонка-дата для подписи).
func maxMinBlock(e *envelope.Envelope) string {
	maxRow, minRow, col, ok := maxMinRows(e)
	if !ok {
		return ""
	}
	dateKey := dateColumn(e)
	if dateKey == "" {
		return ""
	}
	return fmt.Sprintf("Макс: %s — <b>%s</b>\nМин: %s — <b>%s</b>\n",
		html.EscapeString(fmt.Sprintf("%v", maxRow[dateKey])),
		html.EscapeString(render.NumberCompact(toF(maxRow[col.Key]), col.Unit, e.Currency)),
		html.EscapeString(fmt.Sprintf("%v", minRow[dateKey])),
		html.EscapeString(render.NumberCompact(toF(minRow[col.Key]), col.Unit, e.Currency)))
}

// maxMinRows находит строки с максимальным и минимальным значением первой числовой колонки.
func maxMinRows(e *envelope.Envelope) (maxRow, minRow map[string]any, col envelope.Column, ok bool) {
	// Ищем первую числовую (не date/string) колонку.
	for _, c := range e.Columns {
		if c.Unit == "date" || c.Unit == "string" || c.Unit == "" {
			continue
		}
		col = c
		break
	}
	if col.Key == "" || len(e.Rows) == 0 {
		return nil, nil, envelope.Column{}, false
	}

	maxRow, minRow = e.Rows[0], e.Rows[0]
	maxVal, minVal := toF(e.Rows[0][col.Key]), toF(e.Rows[0][col.Key])
	for _, row := range e.Rows[1:] {
		v := toF(row[col.Key])
		if v > maxVal {
			maxVal, maxRow = v, row
		}
		if v < minVal {
			minVal, minRow = v, row
		}
	}
	return maxRow, minRow, col, true
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

// typeEmoji — визуальный маркер типа отчёта в заголовке сообщения.
func typeEmoji(t string) string {
	t = strings.TrimSuffix(strings.TrimSuffix(t, "_compare"), "_contribution")
	switch t {
	case "payment":
		return "💰"
	case "products":
		return "📦"
	case "paycheck":
		return "🧾"
	case "personnel":
		return "👥"
	case "orders":
		return "📋"
	case "forecast":
		return "📈"
	default:
		return "📊"
	}
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
