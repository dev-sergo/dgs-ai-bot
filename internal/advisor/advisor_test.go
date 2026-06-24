package advisor

import (
	"strings"
	"testing"

	"dgsbot/internal/engine"
	"dgsbot/internal/envelope"
	"dgsbot/internal/llm"
)

func ptr(f float64) *float64 { return &f }

func TestCompose(t *testing.T) {
	b := engine.InsightBundle{
		Currency:      "RUB",
		Revenue:       engine.Trend{Now: 1500, Prev: 2000, DeltaAbs: -500, DeltaPct: -25},
		ReturnsSum:    engine.Trend{Now: 200},
		ReturnCount:   3,
		ReturnRate:    13.33,
		Discounts:     105,
		DiscountShare: 7,
		BottomProducts: []engine.NamedRow{
			{Name: "Вода", Amount: 150, Quantity: 5, Profit: ptr(-15)},
			{Name: "Кофе", Amount: 300, Quantity: 4, Profit: ptr(120)},
		},
	}
	out := Compose(b)

	// Совет должен опираться на числа и называть драйверы потерь + аутсайдеры,
	// в т.ч. относительную тяжесть потерь (доля выручки).
	for _, want := range []string{"Выручка", "снижение", "возвраты", "скидки", "Вода", "в минусе",
		"13.33% выручки", "7% выручки"} {
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

// Advisor падает на детерминированный Compose, когда модель сорвалась в китайский
// (общий детектор llm.HasNonRussian, roadmap 5.5).
func TestAdvisorRejectsNonRussian(t *testing.T) {
	if llm.HasNonRussian("Выручка снизилась на 500 ₽ (−25 %), возвраты — 200 ₽.") {
		t.Error("нормальный русский текст совета ошибочно отбракован")
	}
	if !llm.HasNonRussian("Прирост выручки主要是由信用卡支付增加3461 RUB驱动") {
		t.Error("совет с китайским не отбракован")
	}
}
