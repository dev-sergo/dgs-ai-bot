// Package http — HTTP-транспорт сервиса (POST /ask, GET /healthz).
package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"dgsbot/internal/app"
	"dgsbot/internal/config"
	"dgsbot/internal/export"
)

// Server — HTTP-сервер сервиса.
type Server struct {
	cfg config.Config
	app *app.App
	srv *http.Server
}

// New создаёт сервер поверх оркестратора.
func New(cfg config.Config, a *app.App) *Server {
	s := &Server{cfg: cfg, app: a}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /ask", s.handleAsk)
	mux.HandleFunc("GET /export", s.handleExport)
	s.srv = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      s.gate(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 90 * time.Second,
	}
	return s
}

// gate — middleware демо-авторизации по общему токену (env AUTH_TOKEN).
// Токен принимается из заголовка X-Auth-Token, Authorization: Bearer <tok>, либо ?key=<tok>.
// Пустой AUTH_TOKEN отключает гейт (dev/CI). /healthz всегда открыт (health-чек туннеля).
func (s *Server) gate(next http.Handler) http.Handler {
	token := s.cfg.AuthToken
	if token == "" {
		log.Printf("WARNING: AUTH_TOKEN не задан — гейт ОТКЛЮЧЁН, сервис открыт всем")
		return next
	}
	log.Printf("auth gate: enabled (token required on all routes except /healthz)")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			next.ServeHTTP(w, r)
			return
		}
		if !tokenOK(r, token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{
				"error": "доступ закрыт: укажите токен (?key=… либо заголовок X-Auth-Token)",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenOK сверяет токен запроса с ожидаемым в постоянном времени.
func tokenOK(r *http.Request, want string) bool {
	got := r.Header.Get("X-Auth-Token")
	if got == "" {
		if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
			got = strings.TrimPrefix(a, "Bearer ")
		}
	}
	if got == "" {
		got = r.URL.Query().Get("key")
	}
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// Run запускает сервер и завершает его по отмене контекста.
func (s *Server) Run(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.srv.Shutdown(shCtx)
	}()
	log.Printf("listening on %s", s.cfg.HTTPAddr)
	if err := s.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type askRequest struct {
	Text     string `json:"text"`
	TenantID string `json:"tenant_id,omitempty"`
}

func (s *Server) handleAsk(w http.ResponseWriter, r *http.Request) {
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ожидается JSON {\"text\": \"...\"}"})
		return
	}

	// tenant_id — server-side. В MVP берём из заголовка/тела (стаб пре-слоя авторизации).
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		tenantID = req.TenantID
	}
	if tenantID == "" {
		tenantID = "mock_single"
	}

	// session_id — ключ памяти диалога (стаб; в проде из пре-слоя). По умолчанию — на тенанта.
	sessionID := r.Header.Get("X-Session-ID")
	if sessionID == "" {
		sessionID = "default:" + tenantID
	}

	ans, err := s.app.Ask(r.Context(), tenantID, sessionID, req.Text)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ans)
}

// handleExport отдаёт отчёт по текстовому запросу как .xlsx (скачивание).
// GET /export?text=<запрос>&tenant_id=&session_id=  — те же данные, что и /ask,
// но сериализованные в Excel. Прогон идёт в отдельной export-сессии, чтобы не
// засорять историю живого диалога.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	text := r.URL.Query().Get("text")
	if text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ожидается ?text=<запрос>"})
		return
	}
	tenantID := firstNonEmpty(r.Header.Get("X-Tenant-ID"), r.URL.Query().Get("tenant_id"), "mock_single")
	sessionID := firstNonEmpty(r.URL.Query().Get("session_id"), "default:"+tenantID)

	ans, err := s.app.Ask(r.Context(), tenantID, "export:"+sessionID, text)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	if ans.Envelope == nil || len(ans.Envelope.Rows) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "нечего экспортировать: это не табличный отчёт или нет данных за период",
		})
		return
	}

	data, err := export.XLSX(ans.Envelope)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "сбор xlsx: " + err.Error()})
		return
	}

	fname := export.Filename(ans.Envelope)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+fname+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}


func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
