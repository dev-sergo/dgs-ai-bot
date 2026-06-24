package dooglys

import (
	"sort"
	"strings"
)

// prodAgg — накопитель по одному товару (ключ — product_id).
type prodAgg struct {
	name     string
	quantity float64
	amount   float64
	discount float64
}

// aggregateProducts строит отчёт «Товары» из order_items заказов, учитывая только
// выручкообразующие заказы (та же логика, что у payment: finished, без возвратов).
// Группировка по product_id; имя — из позиции (тот же источник, что увидит резолвер).
// Колонки: name, quantity, amount (выручка позиции), discount_sum. Прибыли в order_items
// нет → колонка profit не заполняется. Период отсекается по дате смены заказа.
func aggregateProducts(orders []order, fromISO, toISO string, filters []QueryFilter, rule PaymentRule) ([]Row, []string, []string) {
	orderKeep, itemKeep, applied, skipped := buildProductFilter(filters)

	prods := map[string]*prodAgg{}
	for _, o := range orders {
		day := shiftDate(o)
		if day < fromISO || day > toISO {
			continue
		}
		// Заказ участвует в выручке: finished и не возврат (как в payment).
		if o.DateReturned != nil && *o.DateReturned > 0 {
			continue
		}
		if !rule.CountStatuses[statusValue(o, rule.StatusField)] {
			continue
		}
		if !orderKeep(o) {
			continue
		}
		for _, it := range o.OrderItems {
			if !itemKeep(it) {
				continue
			}
			key := it.ProductID
			if key == "" {
				key = it.ProductName
			}
			a := prods[key]
			if a == nil {
				a = &prodAgg{name: it.ProductName}
				prods[key] = a
			}
			a.quantity += it.Quantity
			a.amount += it.TotalCost
			a.discount += it.DiscountValue
		}
	}

	rows := make([]Row, 0, len(prods))
	for _, a := range prods {
		rows = append(rows, Row{
			"name":         a.name,
			"quantity":     round2(a.quantity),
			"amount":       round2(a.amount),
			"discount_sum": round2(a.discount),
		})
	}
	// Стабильный порядок: по выручке убыв., затем по имени (детерминизм при равных).
	sort.SliceStable(rows, func(i, j int) bool {
		ai, aj := rows[i]["amount"].(float64), rows[j]["amount"].(float64)
		if ai != aj {
			return ai > aj
		}
		return rows[i]["name"].(string) < rows[j]["name"].(string)
	})
	return rows, applied, skipped
}

// buildProductFilter делит фильтры на уровень заказа (sale_point, user) и уровень
// позиции (product). product матчится по UUID (product_id) если резолвнут, иначе по имени.
func buildProductFilter(filters []QueryFilter) (orderKeep func(order) bool, itemKeep func(orderItem) bool, applied, skipped []string) {
	var orderPreds []func(order) bool
	var itemPreds []func(orderItem) bool
	for _, f := range filters {
		switch f.Param {
		case "sale_point_id":
			if len(f.UUIDs) == 0 {
				skipped = append(skipped, f.Field)
				continue
			}
			set := toSet(f.UUIDs)
			orderPreds = append(orderPreds, func(o order) bool { return set[o.SalePointID] })
			applied = append(applied, f.Field)
		case "user_id":
			if len(f.UUIDs) == 0 {
				skipped = append(skipped, f.Field)
				continue
			}
			set := toSet(f.UUIDs)
			orderPreds = append(orderPreds, func(o order) bool { return set[o.UserID] })
			applied = append(applied, f.Field)
		case "product_id":
			if len(f.UUIDs) > 0 {
				set := toSet(f.UUIDs)
				itemPreds = append(itemPreds, func(it orderItem) bool { return set[it.ProductID] })
			} else {
				set := toSetLower(f.Names)
				itemPreds = append(itemPreds, func(it orderItem) bool { return set[strings.ToLower(it.ProductName)] })
			}
			applied = append(applied, f.Field)
		default:
			skipped = append(skipped, f.Field) // напр. product_category — в order_items нет
		}
	}
	orderKeep = func(o order) bool {
		for _, p := range orderPreds {
			if !p(o) {
				return false
			}
		}
		return true
	}
	itemKeep = func(it orderItem) bool {
		for _, p := range itemPreds {
			if !p(it) {
				return false
			}
		}
		return true
	}
	return orderKeep, itemKeep, applied, skipped
}
