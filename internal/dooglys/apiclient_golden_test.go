package dooglys

import (
	"encoding/json"
	"os"
	"testing"
)

// Golden-тест: адаптер прогоняется на РЕАЛЬНОМ по форме ответе /sales/order/list
// (боевые заказы тест-тенанта за май 2025, PII отредактированы; полная структура
// сохранена — заодно проверяем, что unmarshal игнорирует лишние поля). Офлайн,
// детерминированно. Итоги выверены против живого API и независимого расчёта.
func TestAggregatePayment_GoldenMay2025(t *testing.T) {
	raw, err := os.ReadFile("testdata/orders_may_2025.json")
	if err != nil {
		t.Fatalf("чтение golden-фикстуры: %v", err)
	}
	var orders []order
	if err := json.Unmarshal(raw, &orders); err != nil {
		t.Fatalf("разбор golden-фикстуры в []order: %v", err)
	}
	if len(orders) != 52 {
		t.Fatalf("ожидали 52 заказа в фикстуре, got %d", len(orders))
	}

	rows, _, _ := aggregatePayment(orders, "2025-05-01", "2025-05-31", nil, DefaultPaymentRule())

	// Сводные итоги по дням — должны совпасть с живым прогоном (10 чеков, net 2608, возвраты 800).
	var checks, net, returns float64
	for _, r := range rows {
		checks += r["kol_vo_chekov"].(float64)
		net += r["sum_all"].(float64)
		returns += r["return_sum"].(float64)
	}
	if checks != 10 {
		t.Errorf("чеков = %v, want 10", checks)
	}
	if net != 2608 {
		t.Errorf("выручка(net) = %v, want 2608", net)
	}
	if returns != 800 {
		t.Errorf("возвраты = %v, want 800", returns)
	}

	// Точечные дни (как в живом smoke).
	if d := rowByDate(rows, "2025-05-12"); d == nil || d["kol_vo_chekov"].(float64) != 2 || d["sum_card"].(float64) != 1256 {
		t.Errorf("2025-05-12: ожидали 2 чека, карта 1256, got %+v", d)
	}
	if d := rowByDate(rows, "2025-05-26"); d == nil || d["kol_vo_chekov"].(float64) != 5 {
		t.Errorf("2025-05-26: ожидали 5 чеков, got %+v", d)
	}
	// Чистый день возврата: net отрицательный, возврат учтён отдельно.
	if d := rowByDate(rows, "2025-05-14"); d == nil || d["return_sum"].(float64) != 300 || d["sum_all"].(float64) != -300 {
		t.Errorf("2025-05-14: ожидали возврат 300 и net -300, got %+v", d)
	}
}
