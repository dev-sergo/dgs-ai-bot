package dooglys

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FixtureClient отдаёт данные из нормализованных фикстур (docs/contracts/fixtures/<report>.json).
// Период применяется к строкам с полем "date". Фильтры применяются там, где в фикстуре есть
// подходящая колонка (см. fieldColumn); иначе фильтр помечается как пропущенный (реальный
// клиент применит его на стороне Dooglys).
type FixtureClient struct {
	dir string
}

// NewFixtureClient создаёт клиента поверх каталога фикстур.
func NewFixtureClient(dir string) *FixtureClient { return &FixtureClient{dir: dir} }

type fixtureFile struct {
	Report string `json:"report"`
	Label  string `json:"label"`
	Rows   []Row  `json:"rows"`
}

const (
	isoLayout = "2006-01-02"
	ruLayout  = "02.01.2006"
)

// fieldColumn — какая колонка фикстуры соответствует фильтру плана.
var fieldColumn = map[string]string{
	"sale_point":   "torgovaya_tochka",
	"payment_type": "tip_oplaty",
	"user":         "kassir",
	"source":       "istochnik",
}

// enumDisplay — машинное значение enum → отображение в данных Dooglys.
var enumDisplay = map[string]string{
	"card": "картой", "cash": "наличные", "online": "онлайн", "sbp": "сбп",
}

func (c *FixtureClient) Fetch(_ context.Context, q Query) (Result, error) {
	path := filepath.Join(c.dir, q.Report+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Result{}, fmt.Errorf("фикстура отчёта %q не найдена: %w", q.Report, err)
	}
	var ff fixtureFile
	if err := json.Unmarshal(raw, &ff); err != nil {
		return Result{}, fmt.Errorf("разбор фикстуры %q: %w", q.Report, err)
	}

	rows := ff.Rows
	if q.From != "" && q.To != "" {
		rows = filterByPeriod(rows, q.From, q.To)
	}

	res := Result{Report: ff.Report, Label: ff.Label}
	for _, f := range q.Filters {
		col := fieldColumn[f.Field]
		if col == "" || !rowsHaveColumn(rows, col) {
			res.FiltersSkipped = append(res.FiltersSkipped, f.Field)
			continue
		}
		rows = applyFilter(rows, col, acceptValues(f))
		res.FiltersApplied = append(res.FiltersApplied, f.Field)
	}
	res.Rows = rows
	return res, nil
}

// acceptValues — допустимые отображаемые значения фильтра (имена + enum-отображения), в lower-case.
func acceptValues(f QueryFilter) map[string]bool {
	set := map[string]bool{}
	for _, n := range f.Names {
		set[strings.ToLower(strings.TrimSpace(n))] = true
		if d, ok := enumDisplay[strings.ToLower(n)]; ok {
			set[d] = true
		}
	}
	return set
}

func applyFilter(rows []Row, col string, accept map[string]bool) []Row {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		v, _ := r[col].(string)
		if accept[strings.ToLower(strings.TrimSpace(v))] {
			out = append(out, r)
		}
	}
	return out
}

func rowsHaveColumn(rows []Row, col string) bool {
	for _, r := range rows {
		if _, ok := r[col]; ok {
			return true
		}
	}
	return false
}

func filterByPeriod(rows []Row, from, to string) []Row {
	fromT, err1 := time.Parse(ruLayout, from)
	toT, err2 := time.Parse(ruLayout, to)
	if err1 != nil || err2 != nil {
		return rows
	}
	hasDate := false
	for _, r := range rows {
		if _, ok := r["date"]; ok {
			hasDate = true
			break
		}
	}
	if !hasDate {
		return rows
	}

	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		ds, ok := r["date"].(string)
		if !ok {
			continue
		}
		d, err := time.Parse(isoLayout, ds)
		if err != nil {
			continue
		}
		if !d.Before(fromT) && !d.After(toT) {
			out = append(out, r)
		}
	}
	return out
}
