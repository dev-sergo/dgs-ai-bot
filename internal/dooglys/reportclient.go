// Report-API клиент Dooglys (x-context auth).
//
// Отличается от APIClient принципиально: нет token-auth, нет агрегации сырых заказов.
// Report-API возвращает готовые server-side агрегаты; клиент только пагинирует
// и конвертирует формат дат. За тот же интерфейс Client — без изменений в ядре.
package dooglys

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// reportPathMap маппит slug отчёта → путь Report-API.
// kitchen/cooks добавится в C2 когда появится фикстура и каталог.
var reportPathMap = map[string]string{
	"personnel": "/report/personnel",
}

// reportLabel — человекочитаемое название отчёта для Result.Label.
var reportLabel = map[string]string{
	"personnel": "Персонал",
}

// reportFilterColumn — какая колонка ответа Report-API соответствует фильтру плана.
// user → name: сотрудник идентифицируется по полю name в строке персонала.
var reportFilterColumn = map[string]string{
	"user": "name",
}

const (
	reportPerPage  = 100
	reportMaxPages = 50
)

// ReportAPIClient реализует Client через Report-API Dooglys.
// Auth: заголовок x-context = JSON {"tenant_id":"...","tenant_domain":"..."}.
// Пагинация: X-Pagination-Page-Count (аналогично APIClient).
// Дата: принимает DD.MM.YYYY, отправляет YYYY-MM-DD (формат Report-API).
type ReportAPIClient struct {
	base string
	xctx string // значение заголовка x-context (сырая JSON-строка)
	http *http.Client
}

// NewReportAPIClient создаёт клиент Report-API.
// base — корень (напр. https://google.dooglys.com или отдельный report-хост).
// xctx — JSON-строка для заголовка x-context.
func NewReportAPIClient(base, xctx string) *ReportAPIClient {
	return &ReportAPIClient{
		base: strings.TrimRight(base, "/"),
		xctx: xctx,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// Fetch реализует Client: тянет отчёт постранично и применяет клиентские фильтры.
func (c *ReportAPIClient) Fetch(ctx context.Context, q Query) (Result, error) {
	path, ok := reportPathMap[q.Report]
	if !ok {
		return Result{}, fmt.Errorf("reportclient: отчёт %q не поддержан", q.Report)
	}

	from, err := ruToISO(q.From)
	if err != nil {
		return Result{}, fmt.Errorf("reportclient: дата от %q: %w", q.From, err)
	}
	to, err := ruToISO(q.To)
	if err != nil {
		return Result{}, fmt.Errorf("reportclient: дата до %q: %w", q.To, err)
	}

	var rows []Row
	for page := 1; page <= reportMaxPages; page++ {
		params := url.Values{}
		params.Set("date_from", from)
		params.Set("date_to", to)
		params.Set("per_page", strconv.Itoa(reportPerPage))
		params.Set("page", strconv.Itoa(page))

		body, pageCount, err := c.getJSON(ctx, c.base+path+"?"+params.Encode())
		if err != nil {
			return Result{}, fmt.Errorf("reportclient %s стр.%d: %w", q.Report, page, err)
		}

		var batch []map[string]any
		if err := json.Unmarshal(body, &batch); err != nil {
			return Result{}, fmt.Errorf("reportclient: разбор %s стр.%d: %w", q.Report, page, err)
		}
		for _, r := range batch {
			rows = append(rows, Row(r))
		}
		if len(batch) == 0 || page >= pageCount {
			break
		}
	}

	res := Result{
		Report: q.Report,
		Label:  reportLabel[q.Report],
	}
	for _, f := range q.Filters {
		col := reportFilterColumn[f.Field]
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

// getJSON делает GET с x-context auth и возвращает тело + X-Pagination-Page-Count.
func (c *ReportAPIClient) getJSON(ctx context.Context, rawURL string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("x-context", c.xctx)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	body, err := readBody(resp)
	if err != nil {
		return nil, 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet(body))
	}
	pageCount, _ := strconv.Atoi(resp.Header.Get("X-Pagination-Page-Count"))
	if pageCount == 0 {
		pageCount = 1
	}
	return body, pageCount, nil
}

// ruToISO конвертирует DD.MM.YYYY → YYYY-MM-DD (формат date_from/date_to Report-API).
func ruToISO(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	t, err := time.Parse("02.01.2006", s)
	if err != nil {
		return "", err
	}
	return t.Format("2006-01-02"), nil
}
