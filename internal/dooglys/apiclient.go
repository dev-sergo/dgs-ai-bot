// Package dooglys — JSON API v1 клиент Dooglys (token-auth).
//
// В отличие от HTMLClient (SSR-скрейпинг, legacy), APIClient ходит в настоящий
// JSON API v1 на том же хосте: get-token по login/password → заголовки
// Tenant-Domain + Access-Token → сырые сущности (заказы), которые мы агрегируем
// сами под отчёты движка. Встаёт за тот же интерфейс Client без изменения ядра.
package dooglys

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "time/tzdata" // встроенная база таймзон — LoadLocation("Europe/Moscow") в distroless
)

// orderListPageSize — сколько заказов тянем за один запрос пагинации.
const orderListPageSize = 200

// orderListMaxPages — страховочный потолок против рантэвей-пагинации.
const orderListMaxPages = 500

// tenantTZ — таймзона тенанта для конвертации границ периода в Unix-фильтр API.
// cashier_shift_date уже приходит готовой ISO-датой смены, поэтому точная группировка
// от TZ не зависит; TZ нужна лишь чтобы корректно очертить окно date_created_from/to.
var tenantTZ = mustLoadTZ("Europe/Moscow")

func mustLoadTZ(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	return loc
}

// APIClient реализует Client через JSON API v1 Dooglys.
type APIClient struct {
	base     string
	domain   string
	login    string
	password string
	http     *http.Client

	// Rule — правило агрегации заказов в отчёт «Выручка». Калибровочные ручки
	// (статусы / база суммы / возвраты); см. DefaultPaymentRule.
	Rule PaymentRule

	mu    sync.Mutex // защищает token
	token string     // кэш персистентного access_token (one_time:false)
}

// NewAPIClient создаёт клиент JSON API v1.
// base — корень тенанта (https://google.dooglys.com); domain — Tenant-Domain ("google").
func NewAPIClient(base, domain, login, password string) *APIClient {
	return &APIClient{
		base:     strings.TrimRight(base, "/"),
		domain:   domain,
		login:    login,
		password: password,
		http:     &http.Client{Timeout: 60 * time.Second},
		Rule:     DefaultPaymentRule(),
	}
}

// Fetch реализует Client. Сейчас поддержан отчёт payment (выручка) поверх сырых
// заказов; остальные отчёты пока приходят из фикстур (см. решение по пилоту —
// на боевых данных только payment).
func (c *APIClient) Fetch(ctx context.Context, q Query) (Result, error) {
	if q.Report != "payment" {
		return Result{}, fmt.Errorf("dooglys apiclient: отчёт %q пока не поддержан API (только payment)", q.Report)
	}

	fromISO, toISO, err := isoRange(q.From, q.To)
	if err != nil {
		return Result{}, fmt.Errorf("dooglys apiclient: период %q-%q: %w", q.From, q.To, err)
	}

	// Окно Unix-фильтра API с запасом ±1 день — точную отсечку делаем по
	// cashier_shift_date уже на нашей стороне (устойчиво к смещениям TZ/смены).
	fromUnix, toUnix := unixWindow(fromISO, toISO)

	orders, err := c.fetchOrders(ctx, fromUnix, toUnix)
	if err != nil {
		return Result{}, fmt.Errorf("dooglys apiclient: %w", err)
	}

	rows, applied, skipped := aggregatePayment(orders, fromISO, toISO, q.Filters, c.Rule)
	return Result{
		Report:         q.Report,
		Label:          "Выручка",
		Rows:           rows,
		FiltersApplied: applied,
		FiltersSkipped: skipped,
	}, nil
}

// fetchOrders тянет все заказы за окно [fromUnix, toUnix] по страницам.
func (c *APIClient) fetchOrders(ctx context.Context, fromUnix, toUnix int64) ([]order, error) {
	var all []order
	for page := 1; page <= orderListMaxPages; page++ {
		params := url.Values{}
		params.Set("per-page", strconv.Itoa(orderListPageSize))
		params.Set("page", strconv.Itoa(page))
		params.Set("date_created_from", strconv.FormatInt(fromUnix, 10))
		params.Set("date_created_to", strconv.FormatInt(toUnix, 10))
		rawURL := c.base + "/api/v1/sales/order/list?" + params.Encode()

		body, pageCount, err := c.getJSON(ctx, rawURL)
		if err != nil {
			return nil, err
		}
		var batch []order
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("разбор order/list (page %d): %w", page, err)
		}
		all = append(all, batch...)

		if len(batch) == 0 || page >= pageCount {
			break
		}
	}
	return all, nil
}

// getJSON выполняет GET с авто-аутентификацией и одной попыткой ре-логина на 401.
// Возвращает тело и x-pagination-page-count (0 если заголовка нет).
func (c *APIClient) getJSON(ctx context.Context, rawURL string) ([]byte, int, error) {
	body, pageCount, status, err := c.doGet(ctx, rawURL)
	if err != nil {
		return nil, 0, err
	}
	if status == http.StatusUnauthorized {
		c.invalidateToken()
		if body, pageCount, status, err = c.doGet(ctx, rawURL); err != nil {
			return nil, 0, err
		}
	}
	if status != http.StatusOK {
		return nil, 0, fmt.Errorf("GET %s → HTTP %d: %s", rawURL, status, snippet(body))
	}
	return body, pageCount, nil
}

// doGet ставит auth-заголовки и выполняет один GET (без ретраев).
func (c *APIClient) doGet(ctx context.Context, rawURL string) (body []byte, pageCount, status int, err error) {
	tok, err := c.ensureToken(ctx)
	if err != nil {
		return nil, 0, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, 0, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Tenant-Domain", c.domain)
	req.Header.Set("Access-Token", tok)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, 0, err
	}
	defer resp.Body.Close()

	raw, err := readBody(resp)
	if err != nil {
		return nil, 0, 0, err
	}
	pc, _ := strconv.Atoi(resp.Header.Get("X-Pagination-Page-Count"))
	return raw, pc, resp.StatusCode, nil
}

// ensureToken возвращает кэшированный токен либо логинится за новым.
func (c *APIClient) ensureToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	tok, err := c.getToken(ctx)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.token = tok
	c.mu.Unlock()
	return tok, nil
}

func (c *APIClient) invalidateToken() {
	c.mu.Lock()
	c.token = ""
	c.mu.Unlock()
}

// getToken логинится через structure/auth/get-token и возвращает access_token.
func (c *APIClient) getToken(ctx context.Context) (string, error) {
	if c.login == "" || c.password == "" {
		return "", fmt.Errorf("apiclient: пустые DGS_LOGIN/DGS_PASSWORD")
	}
	payload, _ := json.Marshal(map[string]string{"login": c.login, "password": c.password})
	rawURL := c.base + "/api/v1/structure/auth/get-token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Tenant-Domain", c.domain)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := readBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get-token → HTTP %d: %s", resp.StatusCode, snippet(raw))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", fmt.Errorf("get-token: разбор ответа: %w", err)
	}
	if tr.AccessToken == "" {
		return "", fmt.Errorf("get-token: пустой access_token в ответе: %s", snippet(raw))
	}
	return tr.AccessToken, nil
}

// readBody читает тело с прозрачной gzip-декомпрессией.
func readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("gzip: %w", err)
		}
		defer gr.Close()
		reader = gr
	}
	return io.ReadAll(reader)
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
