package dooglys

// contract_test.go — швы «наш код ↔ Dooglys API».
//
// Три проверяемых шва:
//  1. Catalog→Aggregator: поля, которые aggregatePayment/aggregateProducts кладут
//     в Row, должны совпадать с ключами полей каталога. Иначе engine находит пустые
//     или отсутствующие ячейки и считает нули.
//  2. Personnel fixture→Catalog: строки personnel.json должны иметь все non-PII ключи
//     каталога. Если Report-API Dooglys переименует поле — тест это поймает.
//  3. order struct ↔ JSON-теги: ключевые поля заказа (processing_status, cashier_shift_date,
//     order_items и др.) парсятся корректно. Если Dooglys переименует поле в API — тест
//     покажет, что aggregatePayment начнёт получать нули.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"dgsbot/internal/catalog"
)

// fixtureDir — путь к docs/contracts/fixtures относительно этого файла.
func fixtureDir(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	// internal/dooglys/contract_test.go → ../../docs/contracts/fixtures
	return filepath.Join(filepath.Dir(file), "..", "..", "docs", "contracts", "fixtures")
}

// ── Шов 1: Catalog→Aggregator ────────────────────────────────────────────────

// TestPaymentFieldContract — каждый non-PII ключ каталога «Выручка» присутствует
// в строках, которые производит aggregatePayment. Если кто-то добавит поле в каталог
// или переименует в агрегаторе — тест упадёт раньше, чем проблема всплывёт в проде.
func TestPaymentFieldContract(t *testing.T) {
	orders := loadGoldenOrders(t)
	rows, _, _ := aggregatePayment(orders, "2025-05-01", "2025-05-31", nil, DefaultPaymentRule())
	if len(rows) == 0 {
		t.Fatal("golden-фикстура вернула 0 строк — проверь testdata/orders_may_2025.json")
	}

	cat := catalog.Default()
	rep, ok := cat.Report("payment")
	if !ok {
		t.Fatal("каталог не содержит payment — catalog.go рассинхронизирован")
	}

	// Собираем union всех ключей, встретившихся в строках агрегатора.
	produced := unionKeys(rows)

	for _, f := range rep.Fields {
		if f.PII {
			continue // PII-поля агрегатор не производит намеренно
		}
		if _, ok := produced[f.Key]; !ok {
			t.Errorf("catalog.payment field %q отсутствует в строках aggregatePayment (produced=%v)",
				f.Key, sortedKeys(produced))
		}
	}
}

// TestProductsFieldContract — аналогично для отчёта «Товары».
// Поле "profit" из каталога намеренно не производится агрегатором (нет в order_items),
// это задокументировано в apiclient_products.go. Исключаем его из проверки.
func TestProductsFieldContract(t *testing.T) {
	orders := loadGoldenOrders(t)
	rows, _, _ := aggregateProducts(orders, "2025-05-01", "2025-05-31", nil, DefaultPaymentRule())
	if len(rows) == 0 {
		t.Fatal("golden-фикстура вернула 0 строк товаров")
	}

	cat := catalog.Default()
	rep, ok := cat.Report("products")
	if !ok {
		t.Fatal("каталог не содержит products")
	}

	// "profit" отсутствует намеренно: в order_items нет маржи.
	knownGaps := map[string]bool{"profit": true}

	produced := unionKeys(rows)
	for _, f := range rep.Fields {
		if f.PII || knownGaps[f.Key] {
			continue
		}
		if _, ok := produced[f.Key]; !ok {
			t.Errorf("catalog.products field %q отсутствует в строках aggregateProducts (produced=%v)",
				f.Key, sortedKeys(produced))
		}
	}
}

// TestReportAPIPaymentAliasContract — после applyColumnAlias строка Report-API payment
// (форма боевого дампа) содержит ВСЕ non-PII ключи каталога «Выручка». Шов 3a: если в
// каталог добавят payment-поле, а в reportColumnAlias забудут алиас — тест упадёт раньше,
// чем движок начнёт считать нули на боевом Report-API.
func TestReportAPIPaymentAliasContract(t *testing.T) {
	// Форма строки боевого Report-API /report/payment (live-дамп 2026-07-01).
	rows := []Row{{
		"date": "2025-06-01", "count": 138.0, "return_count": 0.0, "return_sum": 0.0,
		"sum_card": 45105.0, "sum_cash": 9206.0, "sum_online": 27585.0, "sum_sbp": 0.0,
		"sum_all": 81896.0, "revenue": 81896.0, "average_sum": 593.45, "average_revenue": 593.45,
		"profit": 47414.28, "payback_count": 0.0, "payback_sum": 0.0,
	}}
	applyColumnAlias("payment", rows)

	cat := catalog.Default()
	rep, ok := cat.Report("payment")
	if !ok {
		t.Fatal("каталог не содержит payment")
	}
	produced := unionKeys(rows)
	for _, f := range rep.Fields {
		if f.PII {
			continue
		}
		if _, ok := produced[f.Key]; !ok {
			t.Errorf("catalog.payment field %q не покрыт Report-API/alias (produced=%v)",
				f.Key, sortedKeys(produced))
		}
	}
}

// ── Шов 2: Personnel fixture→Catalog ─────────────────────────────────────────

// TestPersonnelFixtureFieldContract — строки personnel.json содержат все non-PII ключи
// каталога «Персонал». Если Report-API Dooglys переименует поле (напр. average_revenue →
// avg_check), фикстуру обновят, и этот тест сразу покажет расхождение с каталогом.
func TestPersonnelFixtureFieldContract(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "personnel.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("чтение personnel.json: %v", err)
	}
	var ff struct {
		Rows []map[string]any `json:"rows"`
	}
	if err := json.Unmarshal(raw, &ff); err != nil {
		t.Fatalf("разбор personnel.json: %v", err)
	}
	if len(ff.Rows) == 0 {
		t.Fatal("personnel.json: rows пуст")
	}

	cat := catalog.Default()
	rep, ok := cat.Report("personnel")
	if !ok {
		t.Fatal("каталог не содержит personnel")
	}

	fixtureKeys := unionKeysMap(ff.Rows)
	for _, f := range rep.Fields {
		if f.PII {
			continue
		}
		if _, ok := fixtureKeys[f.Key]; !ok {
			t.Errorf("catalog.personnel field %q отсутствует в personnel.json (fixture keys=%v)",
				f.Key, sortedKeys(fixtureKeys))
		}
	}
}

// TestPersonnelFixtureNoExtraFields — обратная проверка: фикстура не содержит полей,
// которых нет в каталоге (предотвращает «тихое» добавление поля Dooglys без обновления каталога).
func TestPersonnelFixtureNoExtraFields(t *testing.T) {
	path := filepath.Join(fixtureDir(t), "personnel.json")
	raw, _ := os.ReadFile(path)
	var ff struct {
		Rows []map[string]any `json:"rows"`
	}
	json.Unmarshal(raw, &ff)
	if len(ff.Rows) == 0 {
		t.Skip("personnel.json пуст — пропускаем")
	}

	cat := catalog.Default()
	rep, _ := cat.Report("personnel")
	catalogKeys := map[string]bool{}
	for _, f := range rep.Fields {
		catalogKeys[f.Key] = true
	}

	for key := range unionKeysMap(ff.Rows) {
		if !catalogKeys[key] {
			t.Errorf("personnel.json содержит поле %q, которого нет в каталоге — обновите catalog.go", key)
		}
	}
}

// ── Шов 3: order struct ↔ JSON-теги ──────────────────────────────────────────

// TestOrderStructJSONContract — struct order правильно парсит все поля, которые
// использует движок. Если Dooglys переименует JSON-ключ (напр. processing_status →
// process_status), нулевой нарсинг приведёт к тому, что aggregatePayment посчитает
// все заказы «не выручкой» — и этот тест это поймает.
func TestOrderStructJSONContract(t *testing.T) {
	// Минимальный пример заказа, покрывающий все поля, которые использует движок.
	const payload = `[{
		"sale_point_id": "sp-uuid-123",
		"user_id":       "user-uuid-456",
		"total_cost":    1500.50,
		"items_cost":    1400.00,
		"delivery_cost": 100.50,
		"payment_type":  "card",
		"order_status":  "completed",
		"processing_status": "finished",
		"cashier_shift_date": "2025-05-12",
		"date_created":  1715500000,
		"date_returned": null,
		"order_items": [
			{
				"product_id":     "prod-uuid-789",
				"product_name":   "Бизнес ланч",
				"quantity":       2.0,
				"price":          700.0,
				"discount_value": 0.0,
				"total_cost":     1400.0
			}
		],
		"extra_field_unknown_to_us": "ignored"
	}]`

	var orders []order
	if err := json.Unmarshal([]byte(payload), &orders); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if len(orders) != 1 {
		t.Fatalf("ожидали 1 заказ, got %d", len(orders))
	}
	o := orders[0]

	// Каждая проверка — контрактное требование к JSON-ключу.
	assertStr(t, "sale_point_id", o.SalePointID, "sp-uuid-123")
	assertStr(t, "user_id", o.UserID, "user-uuid-456")
	assertF64(t, "total_cost", o.TotalCost, 1500.50)
	assertF64(t, "items_cost", o.ItemsCost, 1400.00)
	assertF64(t, "delivery_cost", o.DeliveryCost, 100.50)
	assertStr(t, "payment_type", o.PaymentType, "card")
	assertStr(t, "order_status", o.OrderStatus, "completed")
	assertStr(t, "processing_status", o.ProcessingStatus, "finished")
	assertStr(t, "cashier_shift_date", o.CashierShiftDate, "2025-05-12")
	if o.DateCreated != 1715500000 {
		t.Errorf("date_created = %d, want 1715500000", o.DateCreated)
	}
	if o.DateReturned != nil {
		t.Errorf("date_returned = %v, want nil", o.DateReturned)
	}
	if len(o.OrderItems) != 1 {
		t.Fatalf("order_items: got %d, want 1", len(o.OrderItems))
	}
	it := o.OrderItems[0]
	assertStr(t, "product_id", it.ProductID, "prod-uuid-789")
	assertStr(t, "product_name", it.ProductName, "Бизнес ланч")
	assertF64(t, "quantity", it.Quantity, 2.0)
	assertF64(t, "price", it.Price, 700.0)
	assertF64(t, "discount_value", it.DiscountValue, 0.0)
	assertF64(t, "item.total_cost", it.TotalCost, 1400.0)
}

// TestOrderStructDateReturned — nullable date_returned: число → *int64, null → nil.
func TestOrderStructDateReturned(t *testing.T) {
	const withReturn = `[{"date_returned": 1715600000}]`
	var orders []order
	json.Unmarshal([]byte(withReturn), &orders)
	if orders[0].DateReturned == nil || *orders[0].DateReturned != 1715600000 {
		t.Errorf("date_returned (number) = %v, want &1715600000", orders[0].DateReturned)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func loadGoldenOrders(t *testing.T) []order {
	t.Helper()
	raw, err := os.ReadFile("testdata/orders_may_2025.json")
	if err != nil {
		t.Fatalf("чтение testdata/orders_may_2025.json: %v", err)
	}
	var orders []order
	if err := json.Unmarshal(raw, &orders); err != nil {
		t.Fatalf("разбор orders_may_2025.json: %v", err)
	}
	return orders
}

func unionKeys(rows []Row) map[string]bool {
	out := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			out[k] = true
		}
	}
	return out
}

func unionKeysMap(rows []map[string]any) map[string]bool {
	out := map[string]bool{}
	for _, r := range rows {
		for k := range r {
			out[k] = true
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	// простой пузырёк — ключей мало, читаемость важнее скорости
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[i] > out[j] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func assertStr(t *testing.T, field, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("JSON field %q: got %q, want %q", field, got, want)
	}
}

func assertF64(t *testing.T, field string, got, want float64) {
	t.Helper()
	if got != want {
		t.Errorf("JSON field %q: got %v, want %v", field, got, want)
	}
}
