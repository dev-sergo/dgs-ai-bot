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
// Уровень A (6 отчётов ТЗ) полностью интегрирован (каталог/нарратор), уровень B —
// транспортно готов: путь есть, но планировщик их НЕ роутит до eval-корпуса (P1).
var reportPathMap = map[string]string{
	// Уровень A — 6 отчётов ТЗ.
	"payment":             "/report/payment",
	"source-order":        "/report/source-order",
	"products":            "/report/products",
	"categories":          "/report/categories",
	"personnel":           "/report/personnel",
	"cash-on-hand":        "/report/cash-on-hand",
	"cash-income-outcome": "/report/cash-income-outcome",
	// Уровень B — транспортно готовы (планировщик НЕ роутит).
	"type-order":            "/report/type-order",
	"expected-profit":       "/report/expected-profit",
	"paycheck":              "/report/paycheck",
	"sales-on-map":          "/report/sales-on-map",
	"order-payment-hour":    "/report/order-payment-hour",
	"special":               "/report/special",
	"special-products":      "/report/special-products",
	"order-processing-time": "/report/order-processing-time",
	"kitchen/cooks":         "/report/kitchen/cooks",
}

// reportLabel — человекочитаемое название отчёта для Result.Label.
var reportLabel = map[string]string{
	"payment":             "Выручка",
	"source-order":        "Источники заказов",
	"products":            "Товары",
	"categories":          "Категории",
	"personnel":           "Персонал",
	"cash-on-hand":        "Наличные в кассе",
	"cash-income-outcome": "Внесения и выплаты",
	// Уровень B.
	"type-order":            "Типы заказов",
	"expected-profit":       "Ожидаемая прибыль",
	"paycheck":              "Чеки",
	"sales-on-map":          "Продажи на карте",
	"order-payment-hour":    "Заказы по часам",
	"special":               "Акции",
	"special-products":      "Товары по акциям",
	"order-processing-time": "Время обработки заказов",
	"kitchen/cooks":         "Кухня (повара)",
}

// reportDefaultSort — обязательный sort_by на отчёт (первое валидное значение enum
// из docs/report.yml). Без него боевой api.dooglys.com отвечает HTTP 400 (sort_by
// помечен required у каждого метода). sort_order по умолчанию reportSortOrder.
var reportDefaultSort = map[string]string{
	"payment":             "date",
	"source-order":        "source",
	"products":            "revenue",
	"categories":          "name",
	"personnel":           "name",
	"cash-on-hand":        "name",
	"cash-income-outcome": "close_date",
	// Уровень B.
	"type-order":            "shift_date",
	"expected-profit":       "date",
	"paycheck":              "number",
	"sales-on-map":          "name",
	"order-payment-hour":    "hour",
	"special":               "name",
	"special-products":      "special_name",
	"order-processing-time": "sale_point_name",
	"kitchen/cooks":         "name",
}

// reportFilterColumn — какая колонка ответа Report-API соответствует фильтру плана.
// user → name: сотрудник идентифицируется по полю name в строке персонала.
// Полное заполнение под 6 отчётов ТЗ — задача 4 (каталог/нарратор).
var reportFilterColumn = map[string]string{
	"user": "name",
}

const (
	reportPerPage   = 100
	reportMaxPages  = 50
	reportSortOrder = "asc" // дефолтный порядок сортировки для всех отчётов
)

// ReportAuthMode — схема авторизации Report-API.
type ReportAuthMode string

const (
	// ReportAuthToken — внешний api.dooglys.com: заголовки access-token + tenant-domain.
	ReportAuthToken ReportAuthMode = "token"
	// ReportAuthXContext — внутренний (в кубах): заголовок x-context (JSON-строка).
	ReportAuthXContext ReportAuthMode = "xcontext"
)

// ReportAuth — креды Report-API для выбранного режима.
type ReportAuth struct {
	Mode         ReportAuthMode
	AccessToken  string // token: значение заголовка access-token
	TenantDomain string // token: значение заголовка tenant-domain
	XContext     string // xcontext: сырая JSON-строка заголовка x-context
}

// ReportAPIClient реализует Client через Report-API Dooglys.
// Auth: два режима (см. ReportAuth) — внешний token или внутренний x-context.
// Пагинация: X-Pagination-Page-Count (аналогично APIClient).
// Дата: принимает DD.MM.YYYY, отправляет YYYY-MM-DD (формат Report-API).
type ReportAPIClient struct {
	base string
	auth ReportAuth
	http *http.Client
}

// newReportAPIClient — общий конструктор; base нормализуется, auth.Mode — обязателен.
func newReportAPIClient(base string, auth ReportAuth) *ReportAPIClient {
	return &ReportAPIClient{
		base: strings.TrimRight(base, "/"),
		auth: auth,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewReportAPIClientToken — внешний режим (api.dooglys.com): access-token + tenant-domain.
// base, как правило, …/api/v1/reports (внешний путь содержит префикс /reports).
func NewReportAPIClientToken(base, accessToken, tenantDomain string) *ReportAPIClient {
	return newReportAPIClient(base, ReportAuth{
		Mode:         ReportAuthToken,
		AccessToken:  accessToken,
		TenantDomain: tenantDomain,
	})
}

// NewReportAPIClientXContext — внутренний режим (в кубах): заголовок x-context.
// base, как правило, …/api/v1 (без префикса /reports).
func NewReportAPIClientXContext(base, xctx string) *ReportAPIClient {
	return newReportAPIClient(base, ReportAuth{
		Mode:     ReportAuthXContext,
		XContext: xctx,
	})
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
		if sortBy := reportDefaultSort[q.Report]; sortBy != "" {
			// sort_by обязателен у боевого API — без него HTTP 400.
			params.Set("sort_by", sortBy)
			params.Set("sort_order", reportSortOrder)
		}
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
	switch c.auth.Mode {
	case ReportAuthToken:
		req.Header.Set("access-token", c.auth.AccessToken)
		req.Header.Set("tenant-domain", c.auth.TenantDomain)
	default: // ReportAuthXContext
		req.Header.Set("x-context", c.auth.XContext)
	}

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
