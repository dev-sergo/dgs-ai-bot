package advisor

import (
	"strings"
	"testing"

	"dgsbot/internal/engine"
	"dgsbot/internal/envelope"
)

func TestCompose(t *testing.T) {
	b := engine.InsightBundle{
		Currency: "RUB",
		Revenue:  engine.Trend{Now: 1500, Prev: 2000, DeltaAbs: -500, DeltaPct: -25},
		ReturnsSum: engine.Trend{Now: 200},
		ReturnCount: 3,
		Discounts:   105,
		BottomProducts: []engine.NamedRow{
			{Name: "Вода", Amount: 150, Quantity: 5, Profit: -15},
			{Name: "Кофе", Amount: 300, Quantity: 4, Profit: 120},
		},
	}
	out := Compose(b)

	// Совет должен опираться на числа и называть драйверы потерь + аутсайдеры.
	for _, want := range []string{"Выручка", "снижение", "возвраты", "скидки", "Вода", "в минусе"} {
		if !strings.Contains(out, want) {
			t.Errorf("в совете нет %q:\n%s", want, out)
		}
	}
	// Прибыльная позиция «Кофе» не должна помечаться как убыточная.
	if strings.Contains(out, "Кофе (300,00 ₽, в минусе)") {
		t.Errorf("прибыльная позиция помечена убыточной:\n%s", out)
	}
}

// Пустой предыдущий период — не считаем проценты, говорим «сравнить не с чем».
func TestCompose_EmptyPrev(t *testing.T) {
	b := engine.InsightBundle{
		Currency: "RUB",
		Revenue:  engine.Trend{Now: 1000, EmptyPrev: true},
		Period:   envelope.Period{From: "2026-06-01", To: "2026-06-07"},
	}
	out := Compose(b)
	if !strings.Contains(out, "сравнить не с чем") {
		t.Errorf("ожидалось упоминание неполного сравнения:\n%s", out)
	}
}

// hasNonRussian отбраковывает срыв в китайский (roadmap 5.5), но пропускает нормальный текст с ₽/%.
func TestHasNonRussian(t *testing.T) {
	if hasNonRussian("Выручка снизилась на 500 ₽ (−25 %), возвраты — 200 ₽.") {
		t.Error("нормальный русский текст ошибочно отбракован")
	}
	if !hasNonRussian("Прирост выручки主要是由信用卡支付增加3461 RUB驱动") {
		t.Error("текст с китайским не отбракован")
	}
}
