// Package resolver — детерминированный резолв имён в uuid по справочникам Dooglys.
//
// LLM выдаёт ИМЕНА (точка «Выкса», товар «Американо»); resolver мэтчит их в uuid,
// которые требуются параметрам BaseReportForm. uuid никогда не приходит от модели.
package resolver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"dgsbot/internal/dooglys"
)

// Entry — запись справочника: один uuid и его возможные имена.
type Entry struct {
	UUID  string
	Names []string
}

// kindSpec описывает, из какого файла и каких колонок брать имена.
type kindSpec struct {
	file     string
	nameCols []int
}

// специфика справочников (под нормализованные grid-фикстуры).
var specs = map[string]kindSpec{
	"sale_point":       {"entities/structure_sale-point.grid.json", []int{0}},
	"locality":         {"entities/structure_locality.grid.json", []int{0, 1}},
	"product":          {"entities/nomenclature_product_index.grid.json", []int{1}},
	"product_category": {"entities/nomenclature_product_index.grid.json", []int{2}}, // категория как имя
	"user":             {"entities/structure_user.grid.json", []int{1, 2}},
}

// Store — загруженные справочники по видам (kind).
//
// byKind защищён mu: индекс товаров может догружаться в фоне (api-режим, SetOptions из
// горутины при старте — см. cmd/server) уже после того, как сервер начал обслуживать
// запросы и звать Resolve. Без блокировки это гонка карты.
type Store struct {
	mu     sync.RWMutex
	byKind map[string][]Entry
}

// Load читает доступные справочники из каталога фикстур.
// Отсутствующие/битые файлы пропускаются (resolver для такого kind вернёт NotFound).
func Load(dir string) *Store {
	s := &Store{byKind: map[string][]Entry{}}
	for kind, sp := range specs {
		entries, err := loadEntries(filepath.Join(dir, sp.file), sp.nameCols)
		if err == nil {
			s.byKind[kind] = entries
		}
	}
	return s
}

// SelectSource — источник живых справочников из HTML-формы отчёта Dooglys.
// Реализуется *dooglys.HTMLClient (метод FetchSelects).
type SelectSource interface {
	FetchSelects(ctx context.Context, report string) (map[string][]dooglys.SelectOption, error)
}

// NewLiveStore строит Store из <select>-данных живой HTML-формы Dooglys: uuid берутся
// прямо из разметки (locality_id/sale_point_id/...), а не из офлайн-снимков grid-фикстур,
// которые устаревают. Используется при DGS_CLIENT=http; для CI/eval остаётся Load (фикстуры).
//
// reports — какие отчёты опросить; по умолчанию payment (его формы достаточно для
// locality+sale_point). kind выводится из имени параметра: locality_id→locality и т.п.
func NewLiveStore(ctx context.Context, src SelectSource, reports ...string) (*Store, error) {
	if len(reports) == 0 {
		reports = []string{"payment"}
	}
	s := &Store{byKind: map[string][]Entry{}}
	seen := map[string]map[string]bool{} // kind → uuid → добавлен
	for _, rep := range reports {
		sel, err := src.FetchSelects(ctx, rep)
		if err != nil {
			return nil, err
		}
		for param, opts := range sel {
			kind := strings.TrimSuffix(param, "_id")
			if _, ok := specs[kind]; !ok {
				continue // вид, который resolver не использует
			}
			if seen[kind] == nil {
				seen[kind] = map[string]bool{}
			}
			for _, o := range opts {
				if o.UUID == "" || o.Name == "" || seen[kind][o.UUID] {
					continue
				}
				seen[kind][o.UUID] = true
				s.byKind[kind] = append(s.byKind[kind], Entry{UUID: o.UUID, Names: []string{o.Name}})
			}
		}
	}
	if len(s.byKind) == 0 {
		return nil, fmt.Errorf("resolver: живая HTML-форма не дала ни одного справочника")
	}
	return s, nil
}

// SetOptions заменяет справочник вида kind живыми записями (uuid+имя). Дедуп по uuid.
// Используется для актуализации списка из API (товары из order_items) поверх фикстур.
func (s *Store) SetOptions(kind string, opts []dooglys.SelectOption) {
	seen := map[string]bool{}
	entries := make([]Entry, 0, len(opts))
	for _, o := range opts {
		if o.UUID == "" || o.Name == "" || seen[o.UUID] {
			continue
		}
		seen[o.UUID] = true
		entries = append(entries, Entry{UUID: o.UUID, Names: []string{o.Name}})
	}
	if len(entries) > 0 {
		s.mu.Lock()
		s.byKind[kind] = entries
		s.mu.Unlock()
	}
}

type gridFile struct {
	Rows []struct {
		Meta  map[string]any `json:"meta"`
		Cells []string       `json:"cells"`
	} `json:"rows"`
}

func loadEntries(path string, nameCols []int) ([]Entry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var g gridFile
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, err
	}
	// product_category повторяется по строкам товаров — дедуп по имени.
	seen := map[string]bool{}
	out := make([]Entry, 0, len(g.Rows))
	for _, r := range g.Rows {
		uuid, _ := r.Meta["key"].(string)
		var names []string
		for _, idx := range nameCols {
			if idx < len(r.Cells) {
				if v := strings.TrimSpace(r.Cells[idx]); v != "" && v != "REDACTED" {
					names = append(names, v)
				}
			}
		}
		if len(names) == 0 {
			continue
		}
		key := strings.ToLower(names[0])
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, Entry{UUID: uuid, Names: names})
	}
	return out, nil
}

// Match — итог резолва одного имени.
type Match struct {
	UUID string
	Name string // каноническое имя из справочника
}

// ErrNotFound / ErrAmbiguous — типизированные ошибки для ветки уточнения.
type ResolveError struct {
	Kind       string
	Query      string
	Ambiguous  bool
	Candidates []string
}

func (e *ResolveError) Error() string {
	if e.Ambiguous {
		return fmt.Sprintf("неоднозначное %q: %s", e.Query, strings.Join(e.Candidates, ", "))
	}
	return fmt.Sprintf("не найдено %q среди %s", e.Query, e.Kind)
}

// Resolve ищет uuid по имени: точное (без регистра), затем подстрочное совпадение.
// Несколько кандидатов → неоднозначность; ноль → не найдено.
func (s *Store) Resolve(kind, name string) (Match, error) {
	// Снимок слайса под RLock; сам слайс/Entry неизменяемы (SetOptions заменяет
	// значение карты целиком), поэтому итерируем уже без блокировки.
	s.mu.RLock()
	entries := s.byKind[kind]
	s.mu.RUnlock()
	q := strings.ToLower(strings.TrimSpace(name))

	// 1) точное совпадение по любому имени.
	for _, e := range entries {
		for _, n := range e.Names {
			if strings.ToLower(n) == q {
				return Match{UUID: e.UUID, Name: e.Names[0]}, nil
			}
		}
	}
	// 2) подстрочное совпадение.
	var hits []Entry
	for _, e := range entries {
		for _, n := range e.Names {
			if strings.Contains(strings.ToLower(n), q) {
				hits = append(hits, e)
				break
			}
		}
	}
	switch len(hits) {
	case 1:
		return Match{UUID: hits[0].UUID, Name: hits[0].Names[0]}, nil
	case 0:
		return Match{}, &ResolveError{Kind: kind, Query: name}
	default:
		cands := make([]string, 0, len(hits))
		for _, h := range hits {
			cands = append(cands, h.Names[0])
		}
		sort.Strings(cands)
		if len(cands) > 5 {
			cands = cands[:5]
		}
		return Match{}, &ResolveError{Kind: kind, Query: name, Ambiguous: true, Candidates: cands}
	}
}
