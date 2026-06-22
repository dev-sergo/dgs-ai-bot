// Package dooglys — слой доступа к данным отчётов.
//
// Контракт один для всех реализаций: (отчёт, период, фильтры) → нормализованные строки.
// FixtureClient читает локальные нормализованные фикстуры; будущий ScrapeClient/APIClient
// встанет за тот же интерфейс без изменения ядра.
package dooglys

import "context"

// Row — нормализованная строка отчёта (числа как float64, остальное как string).
type Row map[string]any

// QueryFilter — один резолвнутый фильтр.
// Names — человекочитаемые значения (для фикстур/отображения),
// UUIDs — резолвнутые идентификаторы (для реального клиента через BaseReportForm[Param]).
type QueryFilter struct {
	Field string
	Param string
	Names []string
	UUIDs []string
}

// Query — запрос к источнику. tenant_id проставляется уже в выполнившем резолв слое.
type Query struct {
	Report  string // slug отчёта
	From    string // DD.MM.YYYY (включительно)
	To      string // DD.MM.YYYY (включительно)
	Filters []QueryFilter
}

// Result — результат выборки.
type Result struct {
	Report         string   `json:"report"`
	Label          string   `json:"label"`
	Rows           []Row    `json:"rows"`
	FiltersApplied []string `json:"filters_applied,omitempty"` // фактически отфильтровали строки
	FiltersSkipped []string `json:"filters_skipped,omitempty"` // нет подходящей колонки в фикстуре
}

// Client — источник данных отчётов.
type Client interface {
	Fetch(ctx context.Context, q Query) (Result, error)
}

// SelectOption — одна запись <select>-фильтра HTML-формы отчёта: живой uuid и его имя.
// Источник актуальных идентификаторов справочников (locality/sale_point/...) прямо из
// разметки Dooglys — в отличие от офлайн-снимков grid-фикстур, которые устаревают.
type SelectOption struct {
	UUID string
	Name string
}
