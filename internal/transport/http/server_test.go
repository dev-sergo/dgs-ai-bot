package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dgsbot/internal/app"
	"dgsbot/internal/config"
)

func gateFor(token string) http.Handler {
	s := &Server{cfg: config.Config{AuthToken: token}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return s.gate(ok)
}

func do(h http.Handler, method, target string, headers map[string]string) int {
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestGate_Disabled_AllowsAll(t *testing.T) {
	h := gateFor("") // пустой токен → гейт выключен
	if code := do(h, "POST", "/ask", nil); code != http.StatusOK {
		t.Errorf("выключенный гейт должен пускать, got %d", code)
	}
}

func TestGate_BlocksWithoutToken(t *testing.T) {
	h := gateFor("secret")
	if code := do(h, "POST", "/ask", nil); code != http.StatusUnauthorized {
		t.Errorf("без токена ожидали 401, got %d", code)
	}
}

func TestGate_HealthAlwaysOpen(t *testing.T) {
	h := gateFor("secret")
	if code := do(h, "GET", "/healthz", nil); code != http.StatusOK {
		t.Errorf("/healthz должен быть открыт, got %d", code)
	}
}

func TestGate_AcceptsTokenForms(t *testing.T) {
	h := gateFor("secret")
	cases := []struct {
		name    string
		target  string
		headers map[string]string
	}{
		{"header", "/ask", map[string]string{"X-Auth-Token": "secret"}},
		{"bearer", "/ask", map[string]string{"Authorization": "Bearer secret"}},
	}
	for _, c := range cases {
		if code := do(h, "POST", c.target, c.headers); code != http.StatusOK {
			t.Errorf("%s: ожидали 200, got %d", c.name, code)
		}
	}
}

// TestGate_RejectsQueryToken: токен из ?key= больше не принимается (утечка в логи URL).
// Верный секрет в URL, но без заголовка → 401.
func TestGate_RejectsQueryToken(t *testing.T) {
	h := gateFor("secret")
	if code := do(h, "POST", "/ask?key=secret", nil); code != http.StatusUnauthorized {
		t.Errorf("токен из ?key= должен быть отвергнут (только заголовок) → 401, got %d", code)
	}
}

func TestGate_RejectsWrongToken(t *testing.T) {
	h := gateFor("secret")
	if code := do(h, "POST", "/ask", map[string]string{"X-Auth-Token": "nope"}); code != http.StatusUnauthorized {
		t.Errorf("неверный токен → 401, got %d", code)
	}
}

func feedbackServer() *Server {
	a := app.New(nil, nil, nil, nil, nil, nil, nil)
	return New(config.Config{}, a)
}

func doBody(h http.Handler, method, target, body string) int {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestFeedback_ValidUp(t *testing.T) {
	s := feedbackServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	if code := doBody(mux, "POST", "/feedback", `{"id":"abc123","rating":"up"}`); code != http.StatusOK {
		t.Errorf("ожидали 200, got %d", code)
	}
}

func TestFeedback_ValidDown(t *testing.T) {
	s := feedbackServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	if code := doBody(mux, "POST", "/feedback", `{"id":"abc123","rating":"down"}`); code != http.StatusOK {
		t.Errorf("ожидали 200, got %d", code)
	}
}

func TestFeedback_MissingID(t *testing.T) {
	s := feedbackServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	if code := doBody(mux, "POST", "/feedback", `{"rating":"up"}`); code != http.StatusBadRequest {
		t.Errorf("без id ожидали 400, got %d", code)
	}
}

func TestFeedback_InvalidRating(t *testing.T) {
	s := feedbackServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	if code := doBody(mux, "POST", "/feedback", `{"id":"abc123","rating":"meh"}`); code != http.StatusBadRequest {
		t.Errorf("неверный rating ожидали 400, got %d", code)
	}
}

func TestFeedback_BadJSON(t *testing.T) {
	s := feedbackServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	if code := doBody(mux, "POST", "/feedback", `not json`); code != http.StatusBadRequest {
		t.Errorf("битый JSON ожидали 400, got %d", code)
	}
}

// TestFeedback_BodyTooLarge: тело > maxBodyBytes отбивается 413 (MaxBytesReader), а не
// прожёвывается целиком в память.
func TestFeedback_BodyTooLarge(t *testing.T) {
	s := feedbackServer()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	huge := `{"id":"` + strings.Repeat("x", maxBodyBytes+1024) + `","rating":"up"}`
	if code := doBody(mux, "POST", "/feedback", huge); code != http.StatusRequestEntityTooLarge {
		t.Errorf("гигантское тело ожидали 413, got %d", code)
	}
}

// TestResolveTenant: server-side проверка tenant_id против реестра.
func TestResolveTenant(t *testing.T) {
	// Пустой реестр (dev/фикстуры) — пермиссив: что передали, то и вернём.
	dev := &Server{tenants: map[string]bool{}}
	if id, ok := dev.resolveTenant(httptest.NewRequest("POST", "/ask", nil), "whatever"); !ok || id != "whatever" {
		t.Errorf("пустой реестр должен пропускать любой tenant, got %q ok=%v", id, ok)
	}

	// Один тенант → default подставляется, когда клиент не указал.
	single := &Server{tenants: map[string]bool{"t1": true}, defaultTenant: "t1"}
	if id, ok := single.resolveTenant(httptest.NewRequest("POST", "/ask", nil), ""); !ok || id != "t1" {
		t.Errorf("single-tenant default: got %q ok=%v, want t1/true", id, ok)
	}

	// Несколько тенантов: известный проходит, произвольный/чужой — отказ.
	multi := &Server{tenants: map[string]bool{"t1": true, "t2": true}}
	if _, ok := multi.resolveTenant(httptest.NewRequest("POST", "/ask", nil), "t2"); !ok {
		t.Error("известный tenant t2 должен проходить")
	}
	if _, ok := multi.resolveTenant(httptest.NewRequest("POST", "/ask", nil), "evil"); ok {
		t.Error("КРОСС-УТЕЧКА: произвольный tenant_id не должен обслуживаться")
	}
	if _, ok := multi.resolveTenant(httptest.NewRequest("POST", "/ask", nil), ""); ok {
		t.Error("multi-tenant без явного tenant_id и без default → отказ")
	}

	// Заголовок X-Tenant-ID имеет приоритет над телом.
	r := httptest.NewRequest("POST", "/ask", nil)
	r.Header.Set("X-Tenant-ID", "t1")
	if id, ok := multi.resolveTenant(r, "t2"); !ok || id != "t1" {
		t.Errorf("заголовок X-Tenant-ID должен побеждать тело, got %q ok=%v", id, ok)
	}
}

// TestHandleAsk_UnknownTenantForbidden: чужой tenant_id отбит 403 ДО вызова app (app.Ask
// не достигается — nil-app не паникует). Дыра изоляции закрыта на входе транспорта.
func TestHandleAsk_UnknownTenantForbidden(t *testing.T) {
	s := &Server{tenants: map[string]bool{"t1": true, "t2": true}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /ask", s.handleAsk)
	if code := doBody(mux, "POST", "/ask", `{"text":"выручка","tenant_id":"evil"}`); code != http.StatusForbidden {
		t.Errorf("чужой tenant_id ожидали 403, got %d", code)
	}
}
