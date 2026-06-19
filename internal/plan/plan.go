// Package plan — контракт между LLM-планировщиком и движком (AnalysisPlan).
package plan

// Class запроса: A — прямой отчёт, B — аналитика (сравнение/вклад).
type Class string

const (
	ClassA Class = "A"
	ClassB Class = "B"
)

// Period — период отчёта. Относительный токен (today/last_7_days/...) или явные даты.
type Period struct {
	Kind  string `json:"kind"`            // "relative" | "explicit"
	Token string `json:"token,omitempty"` // для relative
	From  string `json:"from,omitempty"`  // DD.MM.YYYY для explicit
	To    string `json:"to,omitempty"`
}

// Filter — один фильтр. Для uuid-полей values содержат ИМЕНА (резолвятся в uuid отдельно).
type Filter struct {
	Field  string   `json:"field"`
	Op     string   `json:"op"` // "in" | "eq" | "range"
	Values []string `json:"values,omitempty"`
	From   string   `json:"from,omitempty"`
	To     string   `json:"to,omitempty"`
}

// Output — желаемый формат вывода.
type Output struct {
	Format string `json:"format"` // "auto" | "text" | "xlsx"
}

// AnalysisPlan — структурированный план, который LLM собирает из white-list.
// tenant_id здесь НЕТ намеренно: он проставляется server-side.
type AnalysisPlan struct {
	Version    string   `json:"version"`
	Class      Class    `json:"class"`
	Report     string   `json:"report"`
	Metrics    []string `json:"metrics,omitempty"`
	GroupBy    []string `json:"group_by,omitempty"`
	Period     Period   `json:"period"`
	CompareTo  *Period  `json:"compare_to,omitempty"`
	Method     string   `json:"method"` // plain|compare|contribution|top_n
	TopN       int      `json:"top_n,omitempty"`
	Filters    []Filter `json:"filters,omitempty"`
	Output     Output   `json:"output"`
	Confidence float64  `json:"confidence"`
}
