package export

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"

	"dgsbot/internal/envelope"
)

func TestXLSX_BuildsReadableSheet(t *testing.T) {
	e := &envelope.Envelope{
		Type:     "payment",
		Period:   envelope.Period{From: "2025-05-01", To: "2025-05-31"},
		Currency: "RUB",
		Columns: []envelope.Column{
			{Key: "date", Label: "Дата", Unit: "date"},
			{Key: "kol_vo_chekov", Label: "Чеки", Unit: "count"},
			{Key: "sum_all", Label: "Выручка", Unit: "RUB"},
		},
		Rows: []map[string]any{
			{"date": "2025-05-12", "kol_vo_chekov": 2.0, "sum_all": 1256.0},
			{"date": "2025-05-13", "kol_vo_chekov": 1.0, "sum_all": 1500.0},
		},
		Summary: map[string]float64{"kol_vo_chekov": 3, "sum_all": 2756},
	}

	data, err := XLSX(e)
	if err != nil {
		t.Fatalf("XLSX: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("пустой файл")
	}

	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("файл не открывается как xlsx: %v", err)
	}
	defer f.Close()

	// Шапка на строке 3.
	if v, _ := f.GetCellValue(sheet, "C3"); v != "Выручка" {
		t.Errorf("C3 = %q, want Выручка", v)
	}
	// Первая строка данных — строка 4.
	if v, _ := f.GetCellValue(sheet, "A4"); v != "2025-05-12" {
		t.Errorf("A4 = %q, want 2025-05-12", v)
	}
	// Денежная колонка форматируется (# ##0.00) — Excel отдаёт отображаемое значение.
	if v, _ := f.GetCellValue(sheet, "C4"); v != "1,256.00" {
		t.Errorf("C4 = %q, want 1,256.00 (денежный формат)", v)
	}
	// Итоговая строка — после данных (строка 6): «Итого» + сумма.
	if v, _ := f.GetCellValue(sheet, "A6"); v != "Итого" {
		t.Errorf("A6 = %q, want Итого", v)
	}
	if v, _ := f.GetCellValue(sheet, "C6"); v != "2,756.00" {
		t.Errorf("C6 = %q, want 2,756.00", v)
	}
}

func TestXLSX_NilEnvelope(t *testing.T) {
	if _, err := XLSX(nil); err == nil {
		t.Fatal("ожидали ошибку на nil envelope")
	}
}
