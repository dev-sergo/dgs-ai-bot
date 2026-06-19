// Package http — HTTP-транспорт сервиса (POST /ask, GET /healthz).
package http

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"dgsbot/internal/app"
	"dgsbot/internal/config"
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
	s.srv = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 90 * time.Second,
	}
	return s
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

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
