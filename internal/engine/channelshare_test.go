package engine

import (
	"strings"
	"testing"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/envelope"
)

func channelRows() dooglys.Result {
	// Карта 500, наличные 300, онлайн 150, СБП 50 → сумма 1000.
	return dooglys.Result{Rows: []dooglys.Row{
		{"sum_card": 300.0, "sum_cash": 200.0, "onlayn": 100.0, "sbp": 50.0},
		{"sum_card": 200.0, "sum_cash": 100.0, "onlayn": 50.0, "sbp": 0.0},
	}}
}

func shareOfRow(t *testing.T, env envelope.Envelope, label string) float64 {
	t.Helper()
	for _, r := range env.Rows {
		if r["channel"] == label {
			v, _ := toFloat(r["share"])
			return v
		}
	}
	t.Fatalf("канал %q не найден в раскладке", label)
	return 0
}

func TestChannelShareCashlessFocus(t *testing.T) {
	period := envelope.Period{From: "01.06.2026", To: "30.06.2026"}
	env := ChannelShare(channelRows(), []string{"sum_card", "onlayn", "sbp"}, "T", "RUB", period)

	// Карта 50%, наличные 30%, онлайн 15%, СБП 5%.
	if got := shareOfRow(t, env, "Карта"); got != 50 {
		t.Errorf("Карта share = %v, want 50", got)
	}
	if got := shareOfRow(t, env, "Наличные"); got != 30 {
		t.Errorf("Наличные share = %v, want 30", got)
	}
	// Безнал = карта+онлайн+СБП = 70%.
	if !strings.Contains(env.Narrative, "Безналичные за период — 70,0% выручки") {
		t.Errorf("нарратив не выделяет безнал 70%%: %q", env.Narrative)
	}
	// Полная структура присутствует в скобках.
	if !strings.Contains(env.Narrative, "карта 50,0%") || !strings.Contains(env.Narrative, "сбп 5,0%") {
		t.Errorf("нарратив не содержит полной структуры: %q", env.Narrative)
	}
	if env.Meta["method"] != "channel_share" {
		t.Errorf("method meta = %v, want channel_share", env.Meta["method"])
	}
}

func TestChannelShareSingleChannel(t *testing.T) {
	period := envelope.Period{From: "01.06.2026", To: "30.06.2026"}
	env := ChannelShare(channelRows(), []string{"sum_cash"}, "T", "RUB", period)
	if !strings.Contains(env.Narrative, "Наличные за период — 30,0% выручки") {
		t.Errorf("нарратив для наличных неверен: %q", env.Narrative)
	}
}

func TestChannelShareNoFocusShowsStructure(t *testing.T) {
	period := envelope.Period{From: "01.06.2026", To: "30.06.2026"}
	env := ChannelShare(channelRows(), nil, "T", "RUB", period)
	if !strings.HasPrefix(env.Narrative, "Структура оплат за период:") {
		t.Errorf("ожидалась общая структура: %q", env.Narrative)
	}
}

func TestChannelShareEmpty(t *testing.T) {
	period := envelope.Period{From: "01.06.2026", To: "30.06.2026"}
	env := ChannelShare(dooglys.Result{}, []string{"sum_card"}, "T", "RUB", period)
	if len(env.Rows) != 0 {
		t.Errorf("ожидалось 0 строк на пустых данных, получено %d", len(env.Rows))
	}
	if env.Narrative != "" {
		t.Errorf("на пустых данных нарратив должен быть пустым (ветка empty-result): %q", env.Narrative)
	}
}
