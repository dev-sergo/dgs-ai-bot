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

func TestRender_TextOnly_EscapesHTML(t *testing.T) {
	ans := app.Answer{Plan: okPlan(), Text: "сравни <май> и июнь"}
	text, _ := Render(ans)
	if strings.Contains(text, "<май>") {
		t.Errorf("текст должен быть HTML-экранирован, got %q", text)
	}
	if !strings.Contains(text, "&lt;май&gt;") {
		t.Errorf("ожидалось экранирование &lt;…&gt;, got %q", text)
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

func TestRender_SmallTwoCol_InlineList(t *testing.T) {
	e := &envelope.Envelope{
		Type:     "products",
		Currency: "RUB",
		Period:   envelope.Period{From: "01.05.2026", To: "31.05.2026", TZ: "Europe/Moscow"},
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

	if !strings.Contains(text, "1. Ролл Калифорния — <b>612 400 ₽</b>") {
		t.Errorf("именованный топ → нумерация + жирная компактная сумма, got:\n%s", text)
	}
	if !strings.Contains(text, "2. Сет Филадельфия") {
		t.Errorf("вторая строка должна иметь ранг 2, got:\n%s", text)
	}
	if !strings.Contains(text, "<b>Товары</b>") {
		t.Errorf("заголовок отчёта должен быть жирным, got:\n%s", text)
	}
	if !strings.Contains(text, "01.05.2026 — 31.05.2026") {
		t.Errorf("период должен присутствовать, got:\n%s", text)
	}
	if strings.Contains(text, "---") {
		t.Error("ASCII-таблиц с разделителями быть не должно (мобильный формат)")
	}
	if doc != nil {
		t.Error("маленькая таблица не должна давать файл")
	}
}

func TestRender_SmallWide_InlineCards(t *testing.T) {
	e := &envelope.Envelope{
		Type:     "paycheck",
		Currency: "RUB",
		Period:   envelope.Period{From: "01.06.2026", To: "30.06.2026", TZ: "Europe/Moscow"},
		Columns: []envelope.Column{
			{Key: "num", Label: "№ чека", Unit: "string"},
			{Key: "terminal", Label: "Терминал", Unit: "string"},
			{Key: "closed", Label: "Закрыт", Unit: "string"},
			{Key: "pay_type", Label: "Тип оплаты", Unit: "string"},
			{Key: "paid", Label: "Оплачено", Unit: "RUB"},
			{Key: "discount", Label: "Скидка", Unit: "RUB"},
			{Key: "profit", Label: "Прибыль", Unit: "RUB"},
		},
		Rows: []map[string]any{
			{"num": "138,00", "terminal": "ТСПИоТ SUNMI", "closed": "18 июн. 19:37",
				"pay_type": "Наличные", "paid": 1256.74, "discount": 0.0, "profit": 796.67},
			{"num": "Корп.", "terminal": "ТС ПИоТ", "closed": "16 июн. 18:23",
				"pay_type": "Наличные", "paid": 1215.0, "discount": 0.0, "profit": 1194.08},
		},
		Summary: map[string]float64{"paid": 2471.74, "profit": 1990.75},
	}
	ans := app.Answer{Plan: okPlan(), Text: "не используется", Envelope: e}
	text, doc := Render(ans)

	if doc != nil {
		t.Error("короткий отчёт (≤ maxInlineRows) показывается карточками без файла")
	}
	if !strings.Contains(text, "<b>№ чека: 138,00</b>") {
		t.Errorf("заголовок карточки — первая колонка жирным, got:\n%s", text)
	}
	if !strings.Contains(text, "Оплачено: 1 256,74 ₽") {
		t.Errorf("значения в карточке форматируются по единицам колонок, got:\n%s", text)
	}
	if strings.Contains(text, "Скидка: 0,00 ₽") || strings.Contains(text, "Скидка: 0 ₽") {
		t.Errorf("нулевая скидка должна скрываться в карточке, got:\n%s", text)
	}
	if !strings.Contains(text, "<b>Итого</b>") || !strings.Contains(text, "Оплачено: 2 471,74 ₽") {
		t.Errorf("итоги из Summary должны присутствовать, got:\n%s", text)
	}
	if strings.Contains(text, "не используется") {
		t.Error("Answer.Text (десктопная таблица) не должен попадать в сообщение при envelope")
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
	rows[3]["sum_all"] = 250000.0
	rows[5]["sum_all"] = 50000.0
	e := &envelope.Envelope{
		Type: "payment", Currency: "RUB",
		Period:  envelope.Period{From: "01.05.2026", To: "31.05.2026", TZ: "Europe/Moscow"},
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
	if !strings.Contains(text, "Выручка: 900 000 ₽") {
		t.Errorf("целый итог — без хвоста ,00, got:\n%s", text)
	}
	if !strings.Contains(text, "Макс: 2026-05-01 — <b>250 000 ₽</b>") {
		t.Errorf("макс должен быть компактным и жирным, got:\n%s", text)
	}
	if !strings.Contains(text, "Мин: 2026-05-01 — <b>50 000 ₽</b>") {
		t.Errorf("мин должен быть компактным и жирным, got:\n%s", text)
	}
	if !strings.Contains(text, "в файле ниже") {
		t.Errorf("сводка должна ссылаться на приложенный файл, got:\n%s", text)
	}
}

func TestRender_Narrative_InHeader(t *testing.T) {
	e := &envelope.Envelope{
		Type: "payment_compare", Currency: "RUB",
		Period:    envelope.Period{From: "01.05.2026", To: "31.05.2026", TZ: "Europe/Moscow"},
		Narrative: "Выручка выросла на 12% к прошлому месяцу.",
		Columns: []envelope.Column{
			{Key: "date", Label: "Дата", Unit: "date"},
			{Key: "sum_all", Label: "Выручка", Unit: "RUB"},
		},
		Rows: []map[string]any{{"date": "2026-05-01", "sum_all": 100000.0}},
	}
	ans := app.Answer{Plan: okPlan(), Envelope: e}
	text, _ := Render(ans)
	if !strings.Contains(text, "Выручка выросла на 12%") {
		t.Errorf("нарратив должен присутствовать в сообщении, got:\n%s", text)
	}
}

func TestRender_NoEnvelope_NilDoc(t *testing.T) {
	ans := app.Answer{Plan: okPlan(), Text: "Совет: пересмотри скидки"}
	_, doc := Render(ans)
	if doc != nil {
		t.Error("advice-ответ без envelope не должен давать файл")
	}
}

func TestRender_Compare_DeltaBadge(t *testing.T) {
	up := &envelope.Envelope{
		Type: "payment_compare", Currency: "RUB",
		Period:  envelope.Period{From: "01.06.2026", To: "30.06.2026", TZ: "Europe/Moscow"},
		Columns: []envelope.Column{{Key: "sale_point", Label: "Точка", Unit: "string"}, {Key: "value_now", Label: "Сейчас", Unit: "RUB"}},
		Rows:    []map[string]any{{"sale_point": "ТТ-1", "value_now": 500000.0}},
		Summary: map[string]float64{"delta_pct": 12.4},
	}
	text, _ := Render(app.Answer{Plan: okPlan(), Envelope: up})
	if !strings.Contains(text, "▲ +12,40 % к предыдущему периоду") {
		t.Errorf("рост → стрелка вверх со знаком +, got:\n%s", text)
	}

	down := &envelope.Envelope{
		Type: "payment_compare", Currency: "RUB",
		Period:  envelope.Period{From: "01.06.2026", To: "30.06.2026", TZ: "Europe/Moscow"},
		Columns: up.Columns,
		Rows:    up.Rows,
		Summary: map[string]float64{"delta_pct": -8},
	}
	text2, _ := Render(app.Answer{Plan: okPlan(), Envelope: down})
	if !strings.Contains(text2, "▼ −8,00 % к предыдущему периоду") {
		t.Errorf("падение → стрелка вниз со знаком −, got:\n%s", text2)
	}
}

func TestRowsWord(t *testing.T) {
	cases := map[int]string{1: "строка", 2: "строки", 5: "строк", 11: "строк", 21: "строка", 22: "строки", 100: "строк"}
	for n, want := range cases {
		if got := rowsWord(n); got != want {
			t.Errorf("rowsWord(%d) = %q, want %q", n, got, want)
		}
	}
}
