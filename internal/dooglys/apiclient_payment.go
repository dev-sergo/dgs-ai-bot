package dooglys

import (
	"math"
	"sort"
	"strings"
	"time"
)

// order — сырой заказ из /sales/order/list (только нужные движку поля).
// Суммы — рубли; даты — Unix timestamp; date_returned nullable.
type order struct {
	SalePointID      string  `json:"sale_point_id"`
	UserID           string  `json:"user_id"`
	TotalCost        float64 `json:"total_cost"` // с доставкой
	ItemsCost        float64 `json:"items_cost"` // только позиции
	DeliveryCost     float64 `json:"delivery_cost"`
	PaymentType      string  `json:"payment_type"`      // cash/card/online/sbp
	OrderStatus      string  `json:"order_status"`      // ordered/completed/canceled/...
	ProcessingStatus string  `json:"processing_status"` // finished/confirmed/canceled/... — статус выручки
	CashierShiftDate string  `json:"cashier_shift_date"` // "YYYY-MM-DD" — дата смены
	DateCreated      int64   `json:"date_created"`
	DateReturned     *int64  `json:"date_returned"`
}

// PaymentRule — правило агрегации заказов в отчёт «Выручка».
// Зафиксировано по ответу заказчика (2026-06-24): выручка = total_cost (с доставкой),
// считаются только processing_status=finished, возвраты вычитаются.
type PaymentRule struct {
	// StatusField — поле статуса заказа, по которому решаем «это выручка».
	StatusField string // "processing_status" (рекоменд.) или "order_status"
	// CountStatuses — значения StatusField, попадающие в выручку.
	CountStatuses map[string]bool
	// WithDelivery: true → сумма заказа = total_cost (с доставкой); false → items_cost.
	WithDelivery bool
	// SubtractReturns: true → Выручка (sum_all) = валовая − сумма возвратов.
	SubtractReturns bool
}

// DefaultPaymentRule — правило по ответу заказчика.
func DefaultPaymentRule() PaymentRule {
	return PaymentRule{
		StatusField:     "processing_status",
		CountStatuses:   map[string]bool{"finished": true},
		WithDelivery:    true,
		SubtractReturns: true,
	}
}

// statusValue возвращает значение статус-поля заказа по правилу.
func statusValue(o order, field string) string {
	if field == "order_status" {
		return o.OrderStatus
	}
	return o.ProcessingStatus
}

// channelColumn — payment_type → колонка отчёта.
var channelColumn = map[string]string{
	"cash":   "sum_cash",
	"card":   "sum_card",
	"online": "onlayn",
	"sbp":    "sbp",
}

// dayAgg — накопитель по одной дате смены.
type dayAgg struct {
	checks                        int
	sumCard, sumCash, onlayn, sbp float64
	sumAll                        float64
	returnCount                   int
	returnSum                     float64
}

// aggregatePayment превращает сырые заказы в строки отчёта payment, сгруппированные
// по дате смены. Окно [fromISO, toISO] отсекается по cashier_shift_date; фильтры
// sale_point/payment_type применяются на нашей стороне. Чистая функция (тестируема без сети).
func aggregatePayment(orders []order, fromISO, toISO string, filters []QueryFilter, rule PaymentRule) ([]Row, []string, []string) {
	keep, applied, skipped := buildFilter(filters)

	days := map[string]*dayAgg{}
	for _, o := range orders {
		day := shiftDate(o)
		if day < fromISO || day > toISO {
			continue
		}
		if !keep(o) {
			continue
		}
		amount := o.ItemsCost
		if rule.WithDelivery {
			amount = o.TotalCost
		}

		// Заказ влияет на отчёт только как возврат либо как выручка. Прочие
		// (confirmed/not_confirmed/cooking…) пропускаем, не создавая пустой день.
		isReturn := o.DateReturned != nil && *o.DateReturned > 0
		isRevenue := !isReturn && rule.CountStatuses[statusValue(o, rule.StatusField)]
		if !isReturn && !isRevenue {
			continue
		}

		a := days[day]
		if a == nil {
			a = &dayAgg{}
			days[day] = a
		}

		// Возврат — отдельная колонка; из выручки вычитается (см. SubtractReturns).
		if isReturn {
			a.returnCount++
			a.returnSum += amount
			// Возврат уменьшает свой канал оплаты, чтобы сумма каналов сходилась
			// с чистой выручкой (нал+карта+онлайн+СБП = sum_all).
			if rule.SubtractReturns {
				switch channelColumn[o.PaymentType] {
				case "sum_cash":
					a.sumCash -= amount
				case "sum_card":
					a.sumCard -= amount
				case "onlayn":
					a.onlayn -= amount
				case "sbp":
					a.sbp -= amount
				}
			}
			continue
		}

		a.checks++
		a.sumAll += amount // валовая выручка (возвраты вычтем на выходе)
		switch channelColumn[o.PaymentType] {
		case "sum_cash":
			a.sumCash += amount
		case "sum_card":
			a.sumCard += amount
		case "onlayn":
			a.onlayn += amount
		case "sbp":
			a.sbp += amount
		}
	}

	dates := make([]string, 0, len(days))
	for d := range days {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	rows := make([]Row, 0, len(dates))
	for _, d := range dates {
		a := days[d]
		net := a.sumAll // валовая выручка
		if rule.SubtractReturns {
			net -= a.returnSum // возвраты вычитаются (по правилу заказчика)
		}
		avg := 0.0
		if a.checks > 0 {
			avg = net / float64(a.checks) // средний оплаченный чек
		}
		rows = append(rows, Row{
			"date":          d,
			"kol_vo_chekov": float64(a.checks),
			"return_count":  float64(a.returnCount),
			"return_sum":    round2(a.returnSum),
			"sum_card":      round2(a.sumCard),
			"sum_cash":      round2(a.sumCash),
			"onlayn":        round2(a.onlayn),
			"sbp":           round2(a.sbp),
			"sum_all":       round2(net),
			"sredniy_chek":  round2(avg),
		})
	}
	return rows, applied, skipped
}

// buildFilter собирает предикат отбора заказов из плановых фильтров и сообщает,
// какие фильтры применены, а какие пропущены (нет резолва / не поддержаны на API).
func buildFilter(filters []QueryFilter) (keep func(order) bool, applied, skipped []string) {
	var preds []func(order) bool
	for _, f := range filters {
		switch f.Param {
		case "sale_point_id":
			if len(f.UUIDs) == 0 {
				skipped = append(skipped, f.Field)
				continue
			}
			set := toSet(f.UUIDs)
			preds = append(preds, func(o order) bool { return set[o.SalePointID] })
			applied = append(applied, f.Field)
		case "payment_type":
			set := toSetLower(f.Names)
			preds = append(preds, func(o order) bool { return set[strings.ToLower(o.PaymentType)] })
			applied = append(applied, f.Field)
		default:
			skipped = append(skipped, f.Field)
		}
	}
	keep = func(o order) bool {
		for _, p := range preds {
			if !p(o) {
				return false
			}
		}
		return true
	}
	return keep, applied, skipped
}

// shiftDate — дата группировки заказа: cashier_shift_date, иначе date_created в TZ тенанта.
func shiftDate(o order) string {
	if o.CashierShiftDate != "" {
		return o.CashierShiftDate
	}
	if o.DateCreated > 0 {
		return time.Unix(o.DateCreated, 0).In(tenantTZ).Format(isoLayout)
	}
	return ""
}

// isoRange конвертирует период DD.MM.YYYY → ISO YYYY-MM-DD (включительно).
func isoRange(from, to string) (string, string, error) {
	ft, err := time.Parse(ruLayout, from)
	if err != nil {
		return "", "", err
	}
	tt, err := time.Parse(ruLayout, to)
	if err != nil {
		return "", "", err
	}
	return ft.Format(isoLayout), tt.Format(isoLayout), nil
}

// unixWindow — границы Unix для API-фильтра date_created с запасом ±1 день.
// Точная отсечка периода — позже по cashier_shift_date.
func unixWindow(fromISO, toISO string) (int64, int64) {
	ft, _ := time.ParseInLocation(isoLayout, fromISO, tenantTZ)
	tt, _ := time.ParseInLocation(isoLayout, toISO, tenantTZ)
	fromUnix := ft.AddDate(0, 0, -1).Unix()
	toUnix := tt.AddDate(0, 0, 2).Unix() // +1 день конца + сутки запаса
	return fromUnix, toUnix
}

func toSet(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

func toSetLower(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[strings.ToLower(strings.TrimSpace(x))] = true
	}
	return m
}

func round2(f float64) float64 { return math.Round(f*100) / 100 }
