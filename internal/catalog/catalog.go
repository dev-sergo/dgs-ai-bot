// Package catalog — декларативный white-list отчётов, метрик, измерений и фильтров.
//
// На M0 каталог зашит в коде для 4 MVP-отчётов. На M2 переключимся на загрузку
// configs/catalog.yaml (сгенерирован из docs/contracts/fixtures/catalog.example.yaml),
// сохранив этот же интерфейс.
package catalog

import (
	"fmt"
	"sort"
	"strings"
)

// Field — каноническое поле отчёта.
type Field struct {
	Key   string // машинный ключ (как в нормализованных фикстурах)
	Label string // человекочитаемое имя
	Unit  string // RUB|count|date|percent|string
	PII   bool   // не отдаётся в LLM/ответ
}

// Filter — поддерживаемый фильтр отчёта.
type Filter struct {
	Field   string   // имя фильтра в плане
	Param   string   // имя параметра BaseReportForm[...]
	Kind    string   // "ref" (имя→uuid) | "enum" | "scalar"
	Enum    []string // допустимые значения для enum
	IsMulti bool
}

// Report — описание отчёта в white-list.
type Report struct {
	Slug    string
	Name    string
	Fields  []Field
	Filters []Filter
}

// Catalog — набор включённых отчётов.
type Catalog struct {
	reports map[string]Report
}

// Default возвращает зашитый каталог 4 MVP-отчётов.
func Default() *Catalog {
	common := []Filter{
		{Field: "locality", Param: "locality_id", Kind: "ref", IsMulti: true},
		{Field: "sale_point", Param: "sale_point_id", Kind: "ref", IsMulti: true},
	}
	paymentType := Filter{Field: "payment_type", Param: "payment_type", Kind: "enum",
		Enum: []string{"card", "cash", "online", "sbp"}, IsMulti: true}

	reps := []Report{
		{
			Slug: "payment", Name: "Выручка",
			Fields: []Field{
				{Key: "date", Label: "Дата", Unit: "date"},
				{Key: "kol_vo_chekov", Label: "Кол-во чеков", Unit: "count"},
				{Key: "return_count", Label: "Возвраты", Unit: "count"},
				{Key: "return_sum", Label: "Сумма возвратов", Unit: "RUB"},
				{Key: "sum_card", Label: "Карта", Unit: "RUB"},
				{Key: "sum_cash", Label: "Наличные", Unit: "RUB"},
				{Key: "onlayn", Label: "Онлайн", Unit: "RUB"},
				{Key: "sbp", Label: "СБП", Unit: "RUB"},
				{Key: "sum_all", Label: "Выручка", Unit: "RUB"},
				{Key: "sredniy_chek", Label: "Средний чек", Unit: "RUB"},
			},
			Filters: common,
		},
		{
			Slug: "products", Name: "Товары",
			Fields: []Field{
				{Key: "name", Label: "Название", Unit: "string"},
				{Key: "quantity", Label: "Кол-во", Unit: "count"},
				{Key: "amount", Label: "Выручка", Unit: "RUB"},
				{Key: "profit", Label: "Ожидаемая прибыль", Unit: "RUB"},
				{Key: "discount_sum", Label: "Сумма скидки", Unit: "RUB"},
			},
			Filters: append(append([]Filter{}, common...),
				Filter{Field: "product", Param: "product_id", Kind: "ref", IsMulti: true},
				Filter{Field: "product_category", Param: "product_category_id", Kind: "ref", IsMulti: true},
				Filter{Field: "user", Param: "user_id", Kind: "ref", IsMulti: true},
				Filter{Field: "include_zero_price", Param: "include_zero_price", Kind: "scalar"},
			),
		},
		{
			Slug: "paycheck", Name: "Чеки",
			Fields: []Field{
				{Key: "number", Label: "№ чека", Unit: "string"},
				{Key: "cashier_name", Label: "Кассир", Unit: "string", PII: true},
				{Key: "terminal_name", Label: "Терминал", Unit: "string"},
				{Key: "close", Label: "Закрыт", Unit: "date"},
				{Key: "tip_oplaty", Label: "Тип оплаты", Unit: "string"},
				{Key: "paid", Label: "Оплачено", Unit: "RUB"},
				{Key: "sell_discount", Label: "Скидка", Unit: "RUB"},
				{Key: "profit", Label: "Прибыль", Unit: "RUB"},
			},
			Filters: append(append([]Filter{}, common...), paymentType,
				Filter{Field: "user", Param: "user_id", Kind: "ref", IsMulti: true},
				Filter{Field: "product", Param: "product_id", Kind: "ref", IsMulti: true},
				Filter{Field: "order_number", Param: "order_number", Kind: "scalar"},
			),
		},
		{
			Slug: "orders", Name: "Заказы",
			Fields: []Field{
				{Key: "number", Label: "№", Unit: "string"},
				{Key: "istochnik", Label: "Источник", Unit: "string"},
				{Key: "torgovaya_tochka", Label: "Торговая точка", Unit: "string"},
				{Key: "open", Label: "Открыт", Unit: "date"},
				{Key: "paid", Label: "Стоимость", Unit: "RUB"},
				{Key: "discount_value", Label: "Скидка", Unit: "RUB"},
				{Key: "status", Label: "Статус", Unit: "string"},
				{Key: "tip_oplaty", Label: "Тип оплаты", Unit: "string"},
				{Key: "kassir", Label: "Кассир", Unit: "string", PII: true},
				{Key: "pokupatel", Label: "Покупатель", Unit: "string", PII: true},
			},
			Filters: append(append([]Filter{}, common...), paymentType,
				Filter{Field: "user", Param: "user_id", Kind: "ref", IsMulti: true},
				Filter{Field: "product", Param: "product_id", Kind: "ref", IsMulti: true},
				Filter{Field: "source", Param: "source", Kind: "enum",
					Enum: []string{"delivery-club", "site", "mobile", "cash"}, IsMulti: true},
				Filter{Field: "order_number", Param: "order_number", Kind: "scalar"},
				Filter{Field: "cost_from", Param: "cost_from", Kind: "scalar"},
				Filter{Field: "cost_to", Param: "cost_to", Kind: "scalar"},
			),
		},
	}

	c := &Catalog{reports: make(map[string]Report, len(reps))}
	for _, r := range reps {
		c.reports[r.Slug] = r
	}
	return c
}

// Report возвращает отчёт по slug.
func (c *Catalog) Report(slug string) (Report, bool) {
	r, ok := c.reports[slug]
	return r, ok
}

// Slugs — отсортированный список включённых отчётов.
func (c *Catalog) Slugs() []string {
	out := make([]string, 0, len(c.reports))
	for s := range c.reports {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// NonPIIFieldKeys — ключи полей отчёта без PII (то, что можно отдавать в LLM/ответ).
func (r Report) NonPIIFieldKeys() []string {
	out := make([]string, 0, len(r.Fields))
	for _, f := range r.Fields {
		if !f.PII {
			out = append(out, f.Key)
		}
	}
	return out
}

// HasField сообщает, есть ли в отчёте non-PII поле с таким ключом.
func (r Report) HasNonPIIField(key string) bool {
	for _, f := range r.Fields {
		if f.Key == key {
			return !f.PII
		}
	}
	return false
}

// FieldByKey возвращает поле отчёта по ключу.
func (r Report) FieldByKey(key string) (Field, bool) {
	for _, f := range r.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return Field{}, false
}

// FilterByField возвращает описание фильтра по имени поля плана.
func (r Report) FilterByField(field string) (Filter, bool) {
	for _, f := range r.Filters {
		if f.Field == field {
			return f, true
		}
	}
	return Filter{}, false
}

// Describe — компактное текстовое описание каталога для промпта планировщика.
func (c *Catalog) Describe() string {
	var b strings.Builder
	for _, slug := range c.Slugs() {
		r := c.reports[slug]
		b.WriteString(fmt.Sprintf("- report %q (%s)\n", r.Slug, r.Name))
		b.WriteString("    metrics: " + strings.Join(r.NonPIIFieldKeys(), ", ") + "\n")
		var fs []string
		for _, f := range r.Filters {
			d := f.Field
			if f.Kind == "enum" {
				d += "[" + strings.Join(f.Enum, "|") + "]"
			}
			fs = append(fs, d)
		}
		b.WriteString("    filters: " + strings.Join(fs, ", ") + "\n")
	}
	return b.String()
}
