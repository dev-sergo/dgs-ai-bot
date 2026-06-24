// Package export — сериализация envelope в файлы для скачивания (.xlsx).
// Числа берутся из envelope (источник истины) — как и текстовый рендер.
package export

import (
	"fmt"

	"github.com/xuri/excelize/v2"

	"dgsbot/internal/envelope"
)

const sheet = "Отчёт"

// XLSX строит .xlsx из envelope: заголовок (период), шапка колонок, строки данных
// и итоговая строка из Summary. Денежные/количественные колонки форматируются числом.
func XLSX(e *envelope.Envelope) ([]byte, error) {
	if e == nil {
		return nil, fmt.Errorf("export: пустой envelope")
	}
	f := excelize.NewFile()
	defer f.Close()
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	// Стили: жирная шапка и денежный формат (# ##0.00) для валютных колонок.
	headStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	moneyStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("#,##0.00")})
	intStyle, _ := f.NewStyle(&excelize.Style{CustomNumFmt: strPtr("#,##0")})

	// Титул: тип отчёта + период.
	title := e.Type
	if e.Period.From != "" {
		title = fmt.Sprintf("%s · %s … %s", e.Type, e.Period.From, e.Period.To)
	}
	f.SetCellValue(sheet, "A1", title)
	titleStyle, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 13}})
	f.SetCellStyle(sheet, "A1", "A1", titleStyle)

	const headerRow = 3
	// Шапка колонок.
	for ci, col := range e.Columns {
		cell, _ := excelize.CoordinatesToCellName(ci+1, headerRow)
		f.SetCellValue(sheet, cell, col.Label)
		f.SetCellStyle(sheet, cell, cell, headStyle)
		// Ширина по длине заголовка (грубая эвристика, читаемо без ручной подгонки).
		colName, _ := excelize.ColumnNumberToName(ci + 1)
		f.SetColWidth(sheet, colName, colName, widthFor(col))
	}

	// Строки данных.
	for ri, row := range e.Rows {
		r := headerRow + 1 + ri
		for ci, col := range e.Columns {
			cell, _ := excelize.CoordinatesToCellName(ci+1, r)
			setCell(f, cell, row[col.Key])
			applyNumFmt(f, cell, col, moneyStyle, intStyle)
		}
	}

	// Итоговая строка из Summary (если есть числовые итоги).
	if len(e.Summary) > 0 {
		r := headerRow + 1 + len(e.Rows)
		for ci, col := range e.Columns {
			cell, _ := excelize.CoordinatesToCellName(ci+1, r)
			if ci == 0 {
				f.SetCellValue(sheet, cell, "Итого")
				f.SetCellStyle(sheet, cell, cell, headStyle)
				continue
			}
			if v, ok := e.Summary[col.Key]; ok {
				f.SetCellValue(sheet, cell, v)
				// Числовой формат (money/int) — отдельным стилем; bold не ставим,
				// т.к. SetCellStyle заменяет стиль целиком и затёр бы числовой формат.
				applyNumFmt(f, cell, col, moneyStyle, intStyle)
			}
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// setCell кладёт значение в ячейку, сохраняя числовой тип (для форматов Excel).
func setCell(f *excelize.File, cell string, v any) {
	if v == nil {
		return // пустая ячейка
	}
	f.SetCellValue(sheet, cell, v)
}

func applyNumFmt(f *excelize.File, cell string, col envelope.Column, money, integer int) {
	switch col.Unit {
	case "RUB", "percent":
		f.SetCellStyle(sheet, cell, cell, money)
	case "count":
		f.SetCellStyle(sheet, cell, cell, integer)
	}
}

func widthFor(col envelope.Column) float64 {
	w := float64(len([]rune(col.Label))) + 2
	switch col.Unit {
	case "RUB":
		if w < 14 {
			w = 14
		}
	case "date":
		if w < 12 {
			w = 12
		}
	case "string":
		if w < 20 {
			w = 20
		}
	}
	return w
}

func strPtr(s string) *string { return &s }

// Filename строит имя файла из типа отчёта и периода (используется обоими транспортами).
func Filename(e *envelope.Envelope) string {
	name := e.Type
	if name == "" {
		name = "report"
	}
	if e.Period.From != "" {
		name += "_" + e.Period.From + "_" + e.Period.To
	}
	return name + ".xlsx"
}
