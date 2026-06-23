package llm

import "testing"

// TestHasNonRussian — детектор срыва модели в чужой алфавит (roadmap 5.5).
func TestHasNonRussian(t *testing.T) {
	clean := []string{
		"Выручка снизилась на 500 ₽ (−25 %), возвраты — 200 ₽.",
		"Скидки — 1 936,65 ₽ (78.35% выручки). Пересмотрите ассортимент.",
		"", // пустой текст — не брак
		"Revenue grew by 10% — mixed RU/EN допустим.",
	}
	for _, s := range clean {
		if HasNonRussian(s) {
			t.Errorf("нормальный текст ошибочно отбракован: %q", s)
		}
	}

	dirty := []string{
		"Прирост выручки主要是由信用卡支付增加3461 RUB驱动", // qwen-срыв в китайский (реальный кейс)
		"возвраты составили 200 ₽ 主要原因是退款增加导致利润下降明显",
	}
	for _, s := range dirty {
		if !HasNonRussian(s) {
			t.Errorf("текст с китайским НЕ отбракован: %q", s)
		}
	}
}
