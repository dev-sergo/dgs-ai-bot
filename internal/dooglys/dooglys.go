// Package dooglys — слой доступа к данным отчётов.
//
// Контракт один для всех реализаций: (отчёт, период, фильтры) → нормализованные строки.
// FixtureClient читает локальные нормализованные фикстуры; будущий ScrapeClient/APIClient
// встанет за тот же интерфейс без изменения ядра.
package dooglys

import "context"

// Row — нормализованная строка отчёта (числа как float64, остальное как string).
type Row map[string]any

// Query — запрос к источнику. tenant_id проставляется уже в выполнившем резолв слое.
type Query struct {
	Report string // slug отчёта
	From   string // DD.MM.YYYY (включительно)
	To     string // DD.MM.YYYY (включительно)
	// Фильтры (точка/сотрудник/…) появятся на M2 вместе с resolver'ом.
}

// Result — результат выборки.
type Result struct {
	Report string `json:"report"`
	Label  string `json:"label"`
	Rows   []Row  `json:"rows"`
}

// Client — источник данных отчётов.
type Client interface {
	Fetch(ctx context.Context, q Query) (Result, error)
}
