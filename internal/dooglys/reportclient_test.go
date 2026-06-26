package dooglys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newPersonnelServer возвращает тестовый HTTP-сервер, имитирующий /report/personnel.
// pages — список страниц (каждая страница = срез строк).
func newPersonnelServer(t *testing.T, xctxWant string, pages [][]map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/report/personnel" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("x-context"); got != xctxWant {
			t.Errorf("x-context: got %q, want %q", got, xctxWant)
		}
		pageStr := r.URL.Query().Get("page")
		page := 1
		if pageStr != "" {
			if p, err := parseInt(pageStr); err == nil {
				page = p
			}
		}
		idx := page - 1
		var rows []map[string]any
		if idx >= 0 && idx < len(pages) {
			rows = pages[idx]
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Pagination-Page-Count", itoa(len(pages)))
		json.NewEncoder(w).Encode(rows)
	}))
}

func TestReportAPIClient_Fetch_Personnel(t *testing.T) {
	xctx := `{"tenant_id":"test-id","tenant_domain":"test"}`
	srv := newPersonnelServer(t, xctx, [][]map[string]any{
		{
			{"name": "Иванов", "revenue": 50000.0, "profit": 12000.0, "total_count": 150.0, "average_check": 333.0},
			{"name": "Петрова", "revenue": 30000.0, "profit": 8000.0, "total_count": 90.0, "average_check": 333.0},
		},
	})
	defer srv.Close()

	cli := NewReportAPIClient(srv.URL, xctx)
	res, err := cli.Fetch(context.Background(), Query{
		Report: "personnel",
		From:   "01.06.2025",
		To:     "30.06.2025",
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if res.Report != "personnel" {
		t.Errorf("Report=%q, want personnel", res.Report)
	}
	if len(res.Rows) != 2 {
		t.Fatalf("len(Rows)=%d, want 2", len(res.Rows))
	}
	if res.Rows[0]["name"] != "Иванов" {
		t.Errorf("Rows[0][name]=%v, want Иванов", res.Rows[0]["name"])
	}
}

func TestReportAPIClient_DateConversion(t *testing.T) {
	var gotDateFrom, gotDateTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotDateFrom = r.URL.Query().Get("date_from")
		gotDateTo = r.URL.Query().Get("date_to")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Pagination-Page-Count", "1")
		json.NewEncoder(w).Encode([]map[string]any{})
	}))
	defer srv.Close()

	cli := NewReportAPIClient(srv.URL, "{}")
	cli.Fetch(context.Background(), Query{Report: "personnel", From: "01.06.2025", To: "30.06.2025"})

	if gotDateFrom != "2025-06-01" {
		t.Errorf("date_from=%q, want 2025-06-01", gotDateFrom)
	}
	if gotDateTo != "2025-06-30" {
		t.Errorf("date_to=%q, want 2025-06-30", gotDateTo)
	}
}

func TestReportAPIClient_Pagination(t *testing.T) {
	pages := [][]map[string]any{
		{{"name": "Иванов", "revenue": 1.0}},
		{{"name": "Петров", "revenue": 2.0}},
		{{"name": "Сидоров", "revenue": 3.0}},
	}
	srv := newPersonnelServer(t, "{}", pages)
	defer srv.Close()

	cli := NewReportAPIClient(srv.URL, "{}")
	res, err := cli.Fetch(context.Background(), Query{Report: "personnel", From: "01.01.2025", To: "31.01.2025"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res.Rows) != 3 {
		t.Errorf("len(Rows)=%d, want 3 (all pages collected)", len(res.Rows))
	}
}

func TestReportAPIClient_UserFilter(t *testing.T) {
	srv := newPersonnelServer(t, "{}", [][]map[string]any{
		{
			{"name": "Иванов", "revenue": 50000.0},
			{"name": "Петрова", "revenue": 30000.0},
			{"name": "Сидоров", "revenue": 20000.0},
		},
	})
	defer srv.Close()

	cli := NewReportAPIClient(srv.URL, "{}")
	res, err := cli.Fetch(context.Background(), Query{
		Report:  "personnel",
		From:    "01.06.2025",
		To:      "30.06.2025",
		Filters: []QueryFilter{{Field: "user", Param: "user_id", Names: []string{"Иванов"}}},
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(res.Rows) != 1 {
		t.Fatalf("len(Rows)=%d, want 1 (only Иванов)", len(res.Rows))
	}
	if res.Rows[0]["name"] != "Иванов" {
		t.Errorf("Rows[0][name]=%v, want Иванов", res.Rows[0]["name"])
	}
	if len(res.FiltersApplied) != 1 || res.FiltersApplied[0] != "user" {
		t.Errorf("FiltersApplied=%v, want [user]", res.FiltersApplied)
	}
}

func TestReportAPIClient_UnsupportedReport(t *testing.T) {
	cli := NewReportAPIClient("http://localhost", "{}")
	_, err := cli.Fetch(context.Background(), Query{Report: "payment"})
	if err == nil {
		t.Fatal("ожидалась ошибка на неподдержанный отчёт")
	}
}

func TestRuToISO(t *testing.T) {
	cases := []struct{ in, want string }{
		{"01.06.2025", "2025-06-01"},
		{"31.12.2024", "2024-12-31"},
		{"", ""},
	}
	for _, c := range cases {
		got, err := ruToISO(c.in)
		if err != nil {
			t.Errorf("ruToISO(%q): %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ruToISO(%q)=%q, want %q", c.in, got, c.want)
		}
	}
}

// helpers — простые конверторы, чтобы не тащить strconv напрямую в closure.
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}
