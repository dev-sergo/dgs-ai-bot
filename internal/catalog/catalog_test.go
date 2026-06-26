package catalog

import (
	"sort"
	"strings"
	"testing"
)

// TestDefaultSlugs — каталог содержит ожидаемые MVP-отчёты.
func TestDefaultSlugs(t *testing.T) {
	c := Default()
	got := c.Slugs()
	want := []string{"orders", "paycheck", "payment", "personnel", "products"}
	if len(got) != len(want) {
		t.Fatalf("slugs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("slugs[%d] = %q, want %q (got %v)", i, got[i], want[i], got)
		}
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("Slugs() must be sorted, got %v", got)
	}
}

// TestDefaultDimIsKnownField — DefaultDim каждого отчёта обязан существовать как
// поле этого же отчёта; иначе план без group_by соберёт таблицу без смысловой колонки.
func TestDefaultDimIsKnownField(t *testing.T) {
	c := Default()
	for _, slug := range c.Slugs() {
		r, _ := c.Report(slug)
		if r.DefaultDim == "" {
			continue
		}
		if _, ok := r.FieldByKey(r.DefaultDim); !ok {
			t.Errorf("report %q: DefaultDim %q is not a known field", slug, r.DefaultDim)
		}
	}
}

// TestFiltersHaveParam — у каждого фильтра задан непустой Param (имя BaseReportForm[...]),
// иначе резолв соберёт запрос с пустым параметром и Dooglys тихо проигнорирует разрез.
func TestFiltersHaveParam(t *testing.T) {
	c := Default()
	for _, slug := range c.Slugs() {
		r, _ := c.Report(slug)
		for _, f := range r.Filters {
			if f.Field == "" {
				t.Errorf("report %q: filter with empty Field", slug)
			}
			if f.Param == "" {
				t.Errorf("report %q: filter %q has empty Param", slug, f.Field)
			}
			if f.Kind == "enum" && len(f.Enum) == 0 {
				t.Errorf("report %q: enum filter %q has no Enum values", slug, f.Field)
			}
			if k := f.Kind; k != "ref" && k != "enum" && k != "scalar" {
				t.Errorf("report %q: filter %q has unknown Kind %q", slug, f.Field, k)
			}
		}
	}
}

// TestReportLookup — Report отдаёт ok=false на неизвестном slug.
func TestReportLookup(t *testing.T) {
	c := Default()
	if _, ok := c.Report("payment"); !ok {
		t.Error("payment must be present")
	}
	if _, ok := c.Report("nonexistent"); ok {
		t.Error("unknown slug must return ok=false")
	}
}

// TestFilterByField — поиск фильтра по имени поля плана.
func TestFilterByField(t *testing.T) {
	r, _ := Default().Report("products")
	if f, ok := r.FilterByField("product_category"); !ok || f.Param != "product_category_id" {
		t.Errorf("FilterByField(product_category) = %+v, ok=%v", f, ok)
	}
	if _, ok := r.FilterByField("nope"); ok {
		t.Error("unknown filter field must return ok=false")
	}
}

// TestSummable — суммировать можно только RUB/count с agg=sum; avg/строки/даты — нет.
func TestSummable(t *testing.T) {
	cases := []struct {
		f    Field
		want bool
	}{
		{Field{Unit: "RUB"}, true},
		{Field{Unit: "count"}, true},
		{Field{Unit: "RUB", Agg: "avg"}, false},   // средний чек суммировать нельзя
		{Field{Unit: "string"}, false},
		{Field{Unit: "date"}, false},
		{Field{Unit: "RUB", Agg: "none"}, false},
	}
	for _, tc := range cases {
		if got := tc.f.Summable(); got != tc.want {
			t.Errorf("Summable(%+v) = %v, want %v", tc.f, got, tc.want)
		}
	}
}

// TestNonPIIFieldKeysExcludesPII — PII-поля (кассир/покупатель) не утекают в LLM/ответ.
func TestNonPIIFieldKeysExcludesPII(t *testing.T) {
	r, _ := Default().Report("orders")
	for _, k := range r.NonPIIFieldKeys() {
		if k == "kassir" || k == "pokupatel" {
			t.Errorf("PII field %q leaked into NonPIIFieldKeys", k)
		}
	}
	if r.HasNonPIIField("kassir") {
		t.Error("HasNonPIIField(kassir) must be false (PII)")
	}
	if !r.HasNonPIIField("number") {
		t.Error("HasNonPIIField(number) must be true")
	}
}

// TestDescribeListsAllReports — описание для промпта упоминает каждый отчёт.
func TestDescribeListsAllReports(t *testing.T) {
	c := Default()
	desc := c.Describe()
	for _, slug := range c.Slugs() {
		if !strings.Contains(desc, `"`+slug+`"`) {
			t.Errorf("Describe() omits report %q", slug)
		}
	}
}
