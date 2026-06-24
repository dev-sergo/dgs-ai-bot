package telegram

import (
	"strings"
	"testing"

	"dgsbot/internal/app"
	"dgsbot/internal/envelope"
	"dgsbot/internal/plan"
)

func okPlan() plan.AnalysisPlan {
	return plan.AnalysisPlan{Intent: "report", Report: "payment"}
}

func TestRender_TextOnly(t *testing.T) {
	ans := app.Answer{Plan: okPlan(), Text: "Выручка за май: 1 000 000 ₽"}
	text, doc := Render(ans)
	if text != ans.Text {
		t.Errorf("got %q, want %q", text, ans.Text)
	}
	if doc != nil {
		t.Error("doc должен быть nil для ответа без таблицы")
	}
}

func TestRender_EmptyEnvelope(t *testing.T) {
	ans := app.Answer{Plan: okPlan(), Text: "Нет данных", Envelope: &envelope.Envelope{}}
	text, doc := Render(ans)
	if text != ans.Text {
		t.Errorf("got %q", text)
	}
	if doc != nil {
		t.Error("пустой envelope → doc nil")
	}
}

func TestRender_SmallTable_Inline(t *testing.T) {
	e := &envelope.Envelope{
		Type:     "products",
		Currency: "RUB",
		Columns: []envelope.Column{
			{Key: "name", Label: "Товар", Unit: "string"},
			{Key: "amount", Label: "Выручка", Unit: "RUB"},
		},
		Rows: []map[string]any{
			{"name": "Ролл Калифорния", "amount": 612400.0},
			{"name": "Сет Филадельфия", "amount": 548100.0},
		},
	}
	ans := app.Answer{Plan: okPlan(), Text: "Топ за май", Envelope: e}
	text, doc := Render(ans)

	if !strings.Contains(text, "Ролл Калифорния") {
		t.Error("inline-блок должен содержать строки таблицы")
	}
	if !strings.Contains(text, "---") {
		t.Error("inline-блок должен содержать разделитель ---")
	}
	if doc != nil {
		t.Error("маленькая таблица не должна давать файл")
	}
}

func TestRender_LargeTable_XlsxDoc(t *testing.T) {
	cols := []envelope.Column{
		{Key: "date", Label: "Дата", Unit: "date"},
		{Key: "sum_all", Label: "Выручка", Unit: "RUB"},
	}
	rows := make([]map[string]any, maxInlineRows+1)
	for i := range rows {
		rows[i] = map[string]any{"date": "2026-05-01", "sum_all": 100000.0}
	}
	e := &envelope.Envelope{
		Type: "payment", Currency: "RUB",
		Columns: cols, Rows: rows,
		Summary: map[string]float64{"sum_all": 900000},
	}
	ans := app.Answer{Plan: okPlan(), Text: "Выручка по дням", Envelope: e}
	text, doc := Render(ans)

	if doc == nil {
		t.Fatal("крупная таблица должна давать xlsx-документ")
	}
	if !strings.HasSuffix(doc.Name, ".xlsx") {
		t.Errorf("имя файла должно оканчиваться на .xlsx, got %q", doc.Name)
	}
	if !strings.Contains(text, "Выручка по дням") {
		t.Error("текст ответа должен присутствовать")
	}
}

func TestRender_WideTable_XlsxDoc(t *testing.T) {
	cols := make([]envelope.Column, maxInlineCols+1)
	for i := range cols {
		cols[i] = envelope.Column{Key: "c", Label: "Col", Unit: "RUB"}
	}
	e := &envelope.Envelope{
		Type: "payment", Currency: "RUB",
		Columns: cols,
		Rows:    []map[string]any{{"c": 1.0}},
	}
	ans := app.Answer{Plan: okPlan(), Text: "Широкий отчёт", Envelope: e}
	_, doc := Render(ans)
	if doc == nil {
		t.Error("широкая таблица (>maxInlineCols колонок) должна давать xlsx-документ")
	}
}

func TestRender_NoEnvelope_NilDoc(t *testing.T) {
	ans := app.Answer{Plan: okPlan(), Text: "Совет: пересмотри скидки"}
	_, doc := Render(ans)
	if doc != nil {
		t.Error("advice-ответ без envelope не должен давать файл")
	}
}
