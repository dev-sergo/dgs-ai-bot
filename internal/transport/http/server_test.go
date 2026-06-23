package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
		{"query", "/ask?key=secret", nil},
		{"header", "/ask", map[string]string{"X-Auth-Token": "secret"}},
		{"bearer", "/ask", map[string]string{"Authorization": "Bearer secret"}},
	}
	for _, c := range cases {
		if code := do(h, "POST", c.target, c.headers); code != http.StatusOK {
			t.Errorf("%s: ожидали 200, got %d", c.name, code)
		}
	}
}

func TestGate_RejectsWrongToken(t *testing.T) {
	h := gateFor("secret")
	if code := do(h, "POST", "/ask?key=nope", nil); code != http.StatusUnauthorized {
		t.Errorf("неверный токен → 401, got %d", code)
	}
}
