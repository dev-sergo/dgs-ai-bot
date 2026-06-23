// Package envelope — единый формат результата (источник истины для рендера).
package envelope

// Column — описание колонки таблицы.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Unit  string `json:"unit"` // RUB|count|date|percent|string
}

// Period — период результата.
type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
	TZ   string `json:"tz"`
}

// Envelope — нормализованный результат любого отчёта/анализа.
// Числа в Summary/Rows — источник истины; нарратив и рендер берут значения отсюда.
type Envelope struct {
	Type      string             `json:"type"`
	TenantID  string             `json:"tenant_id"`
	Period    Period             `json:"period"`
	Currency  string             `json:"currency"`
	Columns   []Column           `json:"columns"`
	Rows      []map[string]any   `json:"rows"`
	Summary   map[string]float64 `json:"summary"`
	Narrative string             `json:"narrative,omitempty"`
	Meta      map[string]any     `json:"meta,omitempty"`
}
