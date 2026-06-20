// Package plan — контракт между LLM-планировщиком и движком (AnalysisPlan).
package plan

// Class запроса: A — прямой отчёт, B — аналитика (сравнение/вклад).
type Class string

const (
	ClassA Class = "A"
	ClassB Class = "B"
)

// EffectiveIntent возвращает интент с дефолтом "report" для пустого значения.
func (p AnalysisPlan) EffectiveIntent() string {
	if p.Intent == "" {
		return "report"
	}
	return p.Intent
}

// IsReport сообщает, является ли запрос запросом данных.
func (p AnalysisPlan) IsReport() bool { return p.EffectiveIntent() == "report" }

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
	Version string `json:"version"`
	// Intent — что вообще хочет пользователь:
	//   "report"    — запрос данных (заполняются report/metrics/period/...);
	//   "help"      — «что ты умеешь» → ответ возможностями;
	//   "smalltalk" — приветствие/болтовня → короткий ответ (см. Reply);
	//   "off_topic" — вне компетенции → вежливый отказ.
	// Пусто трактуется как "report" (обратная совместимость).
	Intent     string   `json:"intent,omitempty"`
	Reply      string   `json:"reply,omitempty"` // текст ответа для non-report интентов
	Class      Class    `json:"class"`
	Report     string   `json:"report"`
	Metrics    []string `json:"metrics,omitempty"`
	GroupBy    []string `json:"group_by,omitempty"`
	Period     Period   `json:"period"`
	CompareTo  *Period  `json:"compare_to,omitempty"`
	Method     string   `json:"method"` // plain|compare|contribution|top_n
	TopN       int      `json:"top_n,omitempty"`
	SortBy     string   `json:"sort_by,omitempty"` // ключ метрики для сортировки (top_n)
	Order      string   `json:"order,omitempty"`   // "desc" (лучшие) | "asc" (худшие)
	Filters    []Filter `json:"filters,omitempty"`
	Output     Output   `json:"output"`
	Confidence float64  `json:"confidence"`
}
