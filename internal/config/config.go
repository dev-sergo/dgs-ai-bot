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
// Одно-тенантный legacy-путь: если список TENANTS пуст, из этих полей синтезируется
// единственный TenantConfig (совместимость с dev/фикстурами и cmd/server).
type Telegram struct {
	Token         string   // TELEGRAM_TOKEN; пусто → бот не запускается
	Allowlist     []int64  // TELEGRAM_ALLOWLIST — числовые chat_id; пусто → открыт всем
	AllowUsers    []string // TELEGRAM_ALLOWLIST — @username'ы (нормализованы: без '@', lower)
	DefaultTenant string   // TELEGRAM_TENANT; tenant по умолчанию для всех чатов
}

// TenantConfig — один тенант мультитенантного развёртывания: свой Telegram-бот,
// свой whitelist и свои креды доступа к данным (Report-API/JSON API).
//
// Шов «3 бота → 1 бот на все тенанты»: топология (какой чат → какой тенант) живёт
// в транспорте (resolveTenant), а НЕ здесь. Здесь — только описание тенанта.
// Секреты (BotToken, AccessToken, XContext) НЕ печатаются в Summary и НЕ уходят в LLM.
type TenantConfig struct {
	ID          string   // tenant_id/domain для tenantctx и реестра (TENANT_<k>_ID; default = ключ)
	BotToken    string   // токен Telegram-бота этого тенанта (TENANT_<k>_BOT_TOKEN); секрет
	Allowlist   []int64  // whitelist по числовому chat_id (TENANT_<k>_ALLOWLIST); пусто → открыт всем
	AllowUsers  []string // whitelist по @username (тот же TENANT_<k>_ALLOWLIST); нормализованы: без '@', lower
	Domain      string   // tenant-domain Report-API (TENANT_<k>_DOMAIN; default = ID)
	AccessToken string   // access-token Report-API (TENANT_<k>_ACCESS_TOKEN); пусто → общий DGS_ACCESS_TOKEN; секрет
	XContext    string   // x-context Report-API (TENANT_<k>_XCONTEXT); секрет
}

// Config — корневая конфигурация.
type Config struct {
	HTTPAddr     string
	PlannerMode  PlannerMode
	FixturesPath string
	// AppEnv различает боевой и dev-режим (env APP_ENV, default dev). В prod включаются
	// строгие инварианты безопасности (напр. непустой allowlist на тенанта); dev/CI/eval
	// сохраняют прежние послабления («пусто = открыт»).
	AppEnv          string
	AuthToken       string // общий токен демо-гейта (env AUTH_TOKEN); пусто → гейт выключен
	QueryLogPath    string // JSONL-датасет вопросов/ответов (env QUERY_LOG_PATH); пусто → лог выключен
	FeedbackLogPath string // JSONL оценок пользователя (env FEEDBACK_LOG_PATH); пусто → лог выключен
	LLM             LLM
	Dooglys         Dooglys
	Telegram        Telegram
	// Tenants — эффективный список тенантов (заполняется в Load). Из ENV TENANTS,
	// либо один синтезированный из legacy Telegram+Dooglys, если TENANTS пуст.
	Tenants []TenantConfig
}

// Load читает конфиг из ENV с разумными дефолтами под локальную разработку.
func Load() Config {
	c := Config{
		HTTPAddr:        env("HTTP_ADDR", ":8088"),
		PlannerMode:     PlannerMode(env("PLANNER_MODE", string(PlannerLLM))),
		FixturesPath:    env("FIXTURES_PATH", "docs/contracts/fixtures"),
		AppEnv:          env("APP_ENV", "dev"),
		AuthToken:       env("AUTH_TOKEN", ""),
		QueryLogPath:    env("QUERY_LOG_PATH", ""),
		FeedbackLogPath: env("FEEDBACK_LOG_PATH", ""),
		LLM: LLM{
			BaseURL:   env("LLM_BASE_URL", "http://172.20.10.2:8080"),
			Model:     env("LLM_MODEL", "qwen2-5-32b-instruct-q4-k-m-ctx-16k-q8-0-kv-t07"),
			APIKey:    env("LLM_API_KEY", ""),
			Timeout:   envDuration("LLM_TIMEOUT", 180*time.Second),
			ForceJSON: envBool("LLM_FORCE_JSON", true),
		},
		Dooglys: Dooglys{
			Mode:        DooglysMode(env("DGS_CLIENT", string(DooglysFixture))),
			Base:        env("DGS_BASE", "https://google.dooglys.com"),
			Cookie:      env("DGS_COOKIE", ""),
			Domain:      env("DGS_DOMAIN", "google"),
			Login:       env("DGS_LOGIN", ""),
			Password:    env("DGS_PASSWORD", ""),
			ReportBase:  env("DGS_REPORT_BASE", ""),
			ReportAuth:  env("DGS_REPORT_AUTH", "token"),
			AccessToken: env("DGS_ACCESS_TOKEN", ""),
			XContext:    env("DGS_XCONTEXT", ""),
		},
		Telegram: Telegram{
			Token:         env("TELEGRAM_TOKEN", ""),
			DefaultTenant: env("TELEGRAM_TENANT", "mock_single"),
		},
	}
	// Смешанный whitelist (chat_id и/или @username) парсится вне литерала — два выхода.
	c.Telegram.Allowlist, c.Telegram.AllowUsers = envAllowlist("TELEGRAM_ALLOWLIST")
	c.Tenants = loadTenants(c)
	return c
}

// loadTenants собирает список тенантов. Основной путь — ENV TENANTS (ключи через
// запятую) + индексированные TENANT_<ключ>_*. Если TENANTS пуст — деградация к одному
// тенанту, синтезированному из legacy Telegram+Dooglys (dev/фикстуры, cmd/server).
//
// Формат (индексированный, чтобы секреты не пихать одной JSON-строкой в ENV):
//
//	TENANTS=a,b,c
//	TENANT_a_ID=rukagreka           # опц., default = ключ; это tenant_id/domain для tenantctx
//	TENANT_a_BOT_TOKEN=123:ABC      # токен @BotFather
//	TENANT_a_ALLOWLIST=@ivan,111    # csv @username и/или chat_id (пусто → бот открыт всем)
//	TENANT_a_DOMAIN=rukagreka       # опц., default = ID; tenant-domain для Report-API
//	TENANT_a_ACCESS_TOKEN=xxx       # опц.; пусто → общий DGS_ACCESS_TOKEN
//	TENANT_a_XCONTEXT={...}         # опц. (внутренний xcontext-режим)
func loadTenants(c Config) []TenantConfig {
	keys := csvFields(os.Getenv("TENANTS"))
	if len(keys) == 0 {
		// Legacy деградация: один тенант из TELEGRAM_*/DGS_*.
		return []TenantConfig{{
			ID:          c.Telegram.DefaultTenant,
			BotToken:    c.Telegram.Token,
			Allowlist:   c.Telegram.Allowlist,
			AllowUsers:  c.Telegram.AllowUsers,
			Domain:      c.Dooglys.Domain,
			AccessToken: c.Dooglys.AccessToken,
			XContext:    c.Dooglys.XContext,
		}}
	}
	out := make([]TenantConfig, 0, len(keys))
	for _, k := range keys {
		p := "TENANT_" + k + "_"
		id := env(p+"ID", k)
		ids, users := envAllowlist(p + "ALLOWLIST")
		out = append(out, TenantConfig{
			ID:          id,
			BotToken:    os.Getenv(p + "BOT_TOKEN"),
			Allowlist:   ids,
			AllowUsers:  users,
			Domain:      env(p+"DOMAIN", id),
			AccessToken: os.Getenv(p + "ACCESS_TOKEN"),
			XContext:    os.Getenv(p + "XCONTEXT"),
		})
	}
	return out
}

// csvFields разбивает "a, b ,c" в ["a","b","c"] (пустые элементы отброшены).
func csvFields(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
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
	return fmt.Sprintf("env=%s addr=%s planner=%s llm=%s model=%s fixtures=%s dooglys=%s querylog=%s feedbacklog=%s auth=%s tenants=%s",
		c.AppEnv, c.HTTPAddr, c.PlannerMode, c.LLM.BaseURL, c.LLM.Model, c.FixturesPath, dgs, qlog, flog, c.Dooglys.ReportAuth, c.tenantsSummary())
}

// tenantsSummary — компактное описание тенантов БЕЗ секретов: id/domain, размер
// whitelist и признак наличия (не значение!) бот-токена и креда авторизации.
func (c Config) tenantsSummary() string {
	if len(c.Tenants) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(c.Tenants))
	for _, t := range c.Tenants {
		cred := "unset"
		if c.Dooglys.ReportAuth == string(dooglysAuthXContext) {
			if t.XContext != "" {
				cred = "set"
			}
		} else if t.AccessToken != "" || c.Dooglys.AccessToken != "" {
			cred = "set"
		}
		parts = append(parts, fmt.Sprintf("%s(domain=%s alw=%d bot=%s cred=%s)",
			t.ID, t.Domain, len(t.Allowlist)+len(t.AllowUsers), setUnset(t.BotToken != ""), cred))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func setUnset(ok bool) string {
	if ok {
		return "set"
	}
	return "unset"
}

// dooglysAuthXContext дублирует dooglys.ReportAuthXContext как строку, чтобы config
// не зависел от пакета dooglys (иначе import-цикл). Значения должны совпадать.
const dooglysAuthXContext = "xcontext"

// Validate проверяет контракт авторизации данных ДО старта транспортов: битый конфиг
// должен падать явным сообщением на старте, а не HTTP 500 на первом запросе (docs/12 §8).
//
// Проверяется только режим api (реальные данные): в fixture/stub Report-API легитимно
// выключен. Каждый тенант обязан иметь кред выбранного ReportAuth (token→access-token,
// xcontext→x-context); access-token может быть общим (DGS_ACCESS_TOKEN) или пер-тенантным.
func (c Config) Validate() error {
	switch c.Dooglys.ReportAuth {
	case "token", dooglysAuthXContext:
	default:
		return fmt.Errorf("DGS_REPORT_AUTH=%q не поддержан (ожидается token|xcontext)", c.Dooglys.ReportAuth)
	}
	if c.Dooglys.Mode != DooglysAPI {
		return nil // fixture/stub — данные из фикстур, креды не нужны
	}
	if len(c.Tenants) == 0 {
		return fmt.Errorf("DGS_CLIENT=api, но не задан ни один тенант (TENANTS или TELEGRAM_TENANT)")
	}
	for _, t := range c.Tenants {
		if c.Dooglys.ReportAuth == dooglysAuthXContext {
			if t.XContext == "" {
				return fmt.Errorf("тенант %q: DGS_REPORT_AUTH=xcontext, но не задан XCONTEXT", t.ID)
			}
			continue
		}
		if t.AccessToken == "" && c.Dooglys.AccessToken == "" {
			return fmt.Errorf("тенант %q: DGS_REPORT_AUTH=token, но не задан ACCESS_TOKEN (пер-тенантный или общий DGS_ACCESS_TOKEN)", t.ID)
		}
	}
	return nil
}

// IsProd — боевой режим (APP_ENV=prod, регистр не важен). Включает строгие инварианты
// безопасности; всё остальное (dev/CI/eval/пусто) считается небоевым.
func (c Config) IsProd() bool { return strings.EqualFold(c.AppEnv, "prod") }

// ValidateTelegram проверяет предпосылки запуска N ботов (cmd/bot): токен бота на каждого
// тенанта, а в проде — ещё и непустой allowlist. Отделено от Validate: cmd/server ботов
// не поднимает.
//
// Строгий allowlist в проде: пустой список молча открывает бота ВСЕМ (bot.go allowed),
// что нарушает требование «только этот человек». В dev «пусто = открыт» остаётся ради
// фикстур/локальной отладки — режим выбирается APP_ENV, а не молчаливым дефолтом.
func (c Config) ValidateTelegram() error {
	if len(c.Tenants) == 0 {
		return fmt.Errorf("не задан ни один тенант — боту нечего запускать")
	}
	for _, t := range c.Tenants {
		if t.BotToken == "" {
			return fmt.Errorf("тенант %q: не задан токен бота (TENANT_<key>_BOT_TOKEN или TELEGRAM_TOKEN)", t.ID)
		}
		if c.IsProd() && len(t.Allowlist) == 0 && len(t.AllowUsers) == 0 {
			return fmt.Errorf("тенант %q: APP_ENV=prod требует непустой allowlist "+
				"(TENANT_<key>_ALLOWLIST или TELEGRAM_ALLOWLIST — @username и/или chat_id) — иначе бот открыт всем", t.ID)
		}
	}
	return nil
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

// envAllowlist парсит смешанный whitelist из csv ("@ivan, 111, maria"): элемент из одних
// цифр → числовой chat_id; любой другой → @username (нормализован normUsername: без '@',
// нижний регистр). Пустое значение → nil,nil (allowlist выключен → бот открыт всем). Смешение
// позволяет вести доступ по @username и/или по chat_id и переключаться правкой .env без кода.
func envAllowlist(key string) (ids []int64, users []string) {
	for _, part := range strings.Split(os.Getenv(key), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseInt(part, 10, 64); err == nil {
			ids = append(ids, n)
			continue
		}
		if u := normUsername(part); u != "" {
			users = append(users, u)
		}
	}
	return ids, users
}

// normUsername нормализует Telegram-username для сравнения: срезает ведущий '@' и приводит
// к нижнему регистру (Telegram отдаёт username без '@', регистр в вводе произвольный).
func normUsername(s string) string {
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s), "@"))
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
