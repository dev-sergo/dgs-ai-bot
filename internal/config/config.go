// Package config — конфигурация сервиса из переменных окружения (с дефолтами).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// PlannerMode выбирает источник планов: реальная LLM или детерминированный стаб.
type PlannerMode string

const (
	PlannerLLM  PlannerMode = "llm"
	PlannerStub PlannerMode = "stub"
)

// DooglysMode выбирает источник данных отчётов.
type DooglysMode string

const (
	// DooglysFixture — локальные JSON-фикстуры (default; детерминированный, без сети).
	DooglysFixture DooglysMode = "fixture"
	// DooglysHTTP — реальный Dooglys через SSR-HTML + Cookie (требует DGS_COOKIE и DGS_BASE).
	DooglysHTTP DooglysMode = "http"
	// DooglysAPI — JSON API v1 (token-auth): get-token по login/password, сырые сущности,
	// агрегация на нашей стороне. Требует DGS_DOMAIN + DGS_LOGIN + DGS_PASSWORD.
	DooglysAPI DooglysMode = "api"
)

// LLM — настройки OpenAI-совместимого эндпоинта (llama.cpp на риге).
type LLM struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
	// ForceJSON включает response_format=json_object. Если билд llama.cpp
	// не поддерживает — выставь LLM_FORCE_JSON=false (полагаемся на parse+repair).
	ForceJSON bool
}

// Dooglys — настройки клиента Dooglys.
//   - http: SSR-HTML + Cookie (legacy/fallback) — Base + Cookie.
//   - api:  JSON API v1 token-auth — Base + Domain + Login + Password.
//   - Report-API (x-context): ортогонален mode; включается когда XContext != "".
type Dooglys struct {
	Mode     DooglysMode
	Base     string // URL тенанта, напр. https://google.dooglys.com
	Cookie   string // http: полное значение заголовка Cookie (из DGS_COOKIE)
	Domain   string // api: Tenant-Domain, напр. "google" (из DGS_DOMAIN)
	Login    string // api: логин для get-token (из DGS_LOGIN)
	Password string // api: пароль для get-token (из DGS_PASSWORD)
	// Report-API: два режима авторизации, выбираются ReportAuth.
	//   - token:    внешний api.dooglys.com — заголовки access-token + tenant-domain.
	//   - xcontext: внутренний (в кубах) — заголовок x-context (JSON-строка).
	ReportBase  string // DGS_REPORT_BASE; пусто → использует Base
	ReportAuth  string // DGS_REPORT_AUTH = token|xcontext (default token)
	AccessToken string // DGS_ACCESS_TOKEN — значение access-token (token-режим); секрет
	XContext    string // DGS_XCONTEXT — JSON {"tenant_id":"...","tenant_domain":"..."} (xcontext-режим); секрет
}

// Telegram — настройки Telegram-транспорта (тонкий адаптер поверх app.Ask).
// Пустой Token → транспорт выключен (как пустой AUTH_TOKEN выключает HTTP-гейт).
type Telegram struct {
	Token         string  // TELEGRAM_TOKEN; пусто → бот не запускается
	Allowlist     []int64 // TELEGRAM_ALLOWLIST (csv chat_id); пусто → открыт всем
	DefaultTenant string  // TELEGRAM_TENANT; tenant по умолчанию для всех чатов
}

// Config — корневая конфигурация.
type Config struct {
	HTTPAddr     string
	PlannerMode  PlannerMode
	FixturesPath string
	AuthToken       string // общий токен демо-гейта (env AUTH_TOKEN); пусто → гейт выключен
	QueryLogPath    string // JSONL-датасет вопросов/ответов (env QUERY_LOG_PATH); пусто → лог выключен
	FeedbackLogPath string // JSONL оценок пользователя (env FEEDBACK_LOG_PATH); пусто → лог выключен
	LLM          LLM
	Dooglys      Dooglys
	Telegram     Telegram
}

// Load читает конфиг из ENV с разумными дефолтами под локальную разработку.
func Load() Config {
	return Config{
		HTTPAddr:     env("HTTP_ADDR", ":8088"),
		PlannerMode:  PlannerMode(env("PLANNER_MODE", string(PlannerLLM))),
		FixturesPath: env("FIXTURES_PATH", "docs/contracts/fixtures"),
		AuthToken:       env("AUTH_TOKEN", ""),
		QueryLogPath:    env("QUERY_LOG_PATH", ""),
		FeedbackLogPath: env("FEEDBACK_LOG_PATH", ""),
		LLM: LLM{
			BaseURL:   env("LLM_BASE_URL", "http://172.20.10.2:8080"),
			Model:     env("LLM_MODEL", "qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07"),
			APIKey:    env("LLM_API_KEY", ""),
			Timeout:   envDuration("LLM_TIMEOUT", 180*time.Second),
			ForceJSON: envBool("LLM_FORCE_JSON", true),
		},
		Dooglys: Dooglys{
			Mode:       DooglysMode(env("DGS_CLIENT", string(DooglysFixture))),
			Base:       env("DGS_BASE", "https://google.dooglys.com"),
			Cookie:     env("DGS_COOKIE", ""),
			Domain:     env("DGS_DOMAIN", "google"),
			Login:      env("DGS_LOGIN", ""),
			Password:   env("DGS_PASSWORD", ""),
			ReportBase:  env("DGS_REPORT_BASE", ""),
			ReportAuth:  env("DGS_REPORT_AUTH", "token"),
			AccessToken: env("DGS_ACCESS_TOKEN", ""),
			XContext:    env("DGS_XCONTEXT", ""),
		},
		Telegram: Telegram{
			Token:         env("TELEGRAM_TOKEN", ""),
			Allowlist:     envInt64CSV("TELEGRAM_ALLOWLIST"),
			DefaultTenant: env("TELEGRAM_TENANT", "mock_single"),
		},
	}
}

// Summary — однострочное описание для логов (без секретов).
func (c Config) Summary() string {
	dgs := string(c.Dooglys.Mode)
	switch c.Dooglys.Mode {
	case DooglysHTTP:
		dgs = "http(" + c.Dooglys.Base + ")"
	case DooglysAPI:
		dgs = "api(" + c.Dooglys.Base + " domain=" + c.Dooglys.Domain + ")"
	}
	qlog := "off"
	if c.QueryLogPath != "" {
		qlog = c.QueryLogPath
	}
	flog := "off"
	if c.FeedbackLogPath != "" {
		flog = c.FeedbackLogPath
	}
	tg := "off"
	if c.Telegram.Token != "" {
		tg = fmt.Sprintf("on(tenant=%s allowlist=%d)", c.Telegram.DefaultTenant, len(c.Telegram.Allowlist))
	}
	return fmt.Sprintf("addr=%s planner=%s llm=%s model=%s fixtures=%s dooglys=%s querylog=%s feedbacklog=%s telegram=%s",
		c.HTTPAddr, c.PlannerMode, c.LLM.BaseURL, c.LLM.Model, c.FixturesPath, dgs, qlog, flog, tg)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch os.Getenv(key) {
	case "1", "true", "TRUE", "yes":
		return true
	case "0", "false", "FALSE", "no":
		return false
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// envInt64CSV парсит список chat_id из csv ("123,456"). Пустое значение или мусор —
// nil (allowlist выключен → бот открыт всем). Нечисловые элементы тихо пропускаются.
func envInt64CSV(key string) []int64 {
	v := os.Getenv(key)
	if v == "" {
		return nil
	}
	var out []int64
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseInt(part, 10, 64); err == nil {
			out = append(out, n)
		}
	}
	return out
}
