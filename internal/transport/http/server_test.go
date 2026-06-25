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
