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

var validRatings = map[string]bool{"up": true, "down": true}

// maxBodyBytes — жёсткий предел тела запроса (1 MiB). Защита от раздувания памяти
// и дешёвого DoS: /ask и /feedback принимают маленький JSON, гигантское тело — отбой 413.
const maxBodyBytes = 1 << 20

// Server — HTTP-сервер сервиса.
type Server struct {
	cfg config.Config
	app *app.App
	srv *http.Server
	// tenants — множество разрешённых tenantID (из cfg.Tenants). Server-side проверка:
	// клиент не может обслужиться под произвольным/чужим tenant_id (дыра изоляции).
	// Пусто → пермиссив (dev/фикстуры: изолировать нечего).
	tenants map[string]bool
	// defaultTenant — tenantID по умолчанию, когда клиент его не передал. Заполняется
	// только при ровно одном сконфигурированном тенанте (single-tenant удобство); при
	// нескольких тенантах клиент ОБЯЗАН указать tenant_id явно.
	defaultTenant string
}

// New создаёт сервер поверх оркестратора.
func New(cfg config.Config, a *app.App) *Server {
	s := &Server{cfg: cfg, app: a, tenants: map[string]bool{}}
	for _, t := range cfg.Tenants {
		s.tenants[t.ID] = true
	}
	if len(cfg.Tenants) == 1 {
		s.defaultTenant = cfg.Tenants[0].ID
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("POST /ask", s.handleAsk)
	mux.HandleFunc("GET /export", s.handleExport)
	mux.HandleFunc("POST /feedback", s.handleFeedback)
	s.srv = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      s.gate(mux),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 90 * time.Second,
	}
	return s
}

// gate — middleware демо-авторизации по общему токену (env AUTH_TOKEN).
// Токен принимается ТОЛЬКО из заголовка (X-Auth-Token или Authorization: Bearer <tok>).
// Приём из ?key= убран намеренно: URL с токеном оседает в логах прокси/сервера и истории.
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
				"error": "доступ закрыт: укажите токен в заголовке X-Auth-Token или Authorization: Bearer",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

// tokenOK сверяет токен запроса с ожидаемым в постоянном времени. Только из заголовка:
// X-Auth-Token либо Authorization: Bearer <tok>. Приём из URL (?key=) убран, чтобы токен
// не утекал в логи.
func tokenOK(r *http.Request, want string) bool {
	got := r.Header.Get("X-Auth-Token")
	if got == "" {
		if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
			got = strings.TrimPrefix(a, "Bearer ")
		}
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
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req askRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if tooLarge(err) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "тело запроса слишком большое"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ожидается JSON {\"text\": \"...\"}"})
		return
	}
	if req.Text == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ожидается JSON {\"text\": \"...\"}"})
		return
	}

	// tenant_id — server-side: клиентское значение сверяется с реестром, чужой/произвольный
	// tenant_id не обслуживается (дыра изоляции). resolveTenant вернёт ok=false → 403.
	tenantID, ok := s.resolveTenant(r, req.TenantID)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "неизвестный или не указанный tenant_id"})
		return
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

// resolveTenant выбирает tenantID (заголовок X-Tenant-ID → тело → default) и сверяет его
// с сконфигурированным реестром. ok=false, если tenant неизвестен: клиент не может выбрать
// произвольный/чужой tenant_id и достать чужие данные. Пустой реестр (dev/фикстуры) —
// пермиссив: изолировать нечего, обслуживаем как есть.
func (s *Server) resolveTenant(r *http.Request, bodyTenant string) (string, bool) {
	id := firstNonEmpty(r.Header.Get("X-Tenant-ID"), bodyTenant, s.defaultTenant)
	if len(s.tenants) == 0 {
		return id, true // dev/фикстуры: реестр не задан, изоляции нет
	}
	if id == "" || !s.tenants[id] {
		return "", false
	}
	return id, true
}

// tooLarge сообщает, что ошибка вызвана превышением maxBodyBytes (MaxBytesReader).
func tooLarge(err error) bool {
	var mbe *http.MaxBytesError
	return errors.As(err, &mbe)
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
	tenantID, ok := s.resolveTenant(r, r.URL.Query().Get("tenant_id"))
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "неизвестный или не указанный tenant_id"})
		return
	}
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

type feedbackRequest struct {
	ID     string `json:"id"`
	Rating string `json:"rating"`
}

func (s *Server) handleFeedback(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if tooLarge(err) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "тело запроса слишком большое"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ожидается JSON {\"id\":\"...\",\"rating\":\"up|down\"}"})
		return
	}
	if req.ID == "" || !validRatings[req.Rating] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id обязателен, rating должен быть up или down"})
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	s.app.RecordFeedback(ts, req.ID, req.Rating, "ui")
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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
