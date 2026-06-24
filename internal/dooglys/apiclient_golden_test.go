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

// Golden-тест товарного отчёта из order_items тех же реальных заказов.
func TestAggregateProducts_GoldenMay2025(t *testing.T) {
	raw, err := os.ReadFile("testdata/orders_may_2025.json")
	if err != nil {
		t.Fatalf("чтение golden-фикстуры: %v", err)
	}
	var orders []order
	if err := json.Unmarshal(raw, &orders); err != nil {
		t.Fatalf("разбор: %v", err)
	}

	rows, _, _ := aggregateProducts(orders, "2025-05-01", "2025-05-31", nil, DefaultPaymentRule())

	if len(rows) != 17 {
		t.Fatalf("ожидали 17 товаров, got %d", len(rows))
	}
	var amount, qty float64
	for _, r := range rows {
		amount += r["amount"].(float64)
		qty += r["quantity"].(float64)
	}
	if round2(amount) != 3408 {
		t.Errorf("суммарная выручка товаров = %v, want 3408", amount)
	}
	if qty != 39 {
		t.Errorf("суммарное кол-во = %v, want 39", qty)
	}
	// Отсортировано по выручке убыв.: первый — самая дорогая пицца (1500).
	if rows[0]["name"] != "Длинное название самой большой пиццы в мире" || rows[0]["amount"].(float64) != 1500 {
		t.Errorf("топ-товар = %v (%v), want пицца 1500", rows[0]["name"], rows[0]["amount"])
	}
}

// Фильтр по имени товара (drill-down) сворачивает отчёт до одной позиции.
func TestAggregateProducts_FilterByName(t *testing.T) {
	raw, _ := os.ReadFile("testdata/orders_may_2025.json")
	var orders []order
	json.Unmarshal(raw, &orders)

	f := []QueryFilter{{Field: "product", Param: "product_id", Names: []string{"Бизнес ланч"}}}
	rows, applied, _ := aggregateProducts(orders, "2025-05-01", "2025-05-31", f, DefaultPaymentRule())
	if len(applied) != 1 || applied[0] != "product" {
		t.Errorf("applied=%v, want [product]", applied)
	}
	if len(rows) != 1 || rows[0]["name"] != "Бизнес ланч" || rows[0]["amount"].(float64) != 300 {
		t.Errorf("ожидали 1 строку «Бизнес ланч» 300, got %+v", rows)
	}
}
