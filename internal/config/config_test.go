package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadDefaults — без env Load даёт документированные дефолты под локалку.
func TestLoadDefaults(t *testing.T) {
	// Изолируем от окружения CI: явно гасим то, что может протечь.
	for _, k := range []string{"HTTP_ADDR", "PLANNER_MODE", "DGS_CLIENT", "LLM_FORCE_JSON", "AUTH_TOKEN", "APP_ENV"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.HTTPAddr != ":8088" {
		t.Errorf("HTTPAddr = %q, want :8088", c.HTTPAddr)
	}
	if c.AppEnv != "dev" || c.IsProd() {
		t.Errorf("AppEnv default = %q (IsProd=%v), want dev/false", c.AppEnv, c.IsProd())
	}
	if c.PlannerMode != PlannerLLM {
		t.Errorf("PlannerMode = %q, want llm", c.PlannerMode)
	}
	if c.Dooglys.Mode != DooglysFixture {
		t.Errorf("Dooglys.Mode = %q, want fixture", c.Dooglys.Mode)
	}
	if c.Dooglys.ReportAuth != "token" {
		t.Errorf("Dooglys.ReportAuth = %q, want token (default)", c.Dooglys.ReportAuth)
	}
	if !c.LLM.ForceJSON {
		t.Error("LLM.ForceJSON default must be true")
	}
	if c.LLM.Timeout != 180*time.Second {
		t.Errorf("LLM.Timeout = %v, want 180s", c.LLM.Timeout)
	}
}

// TestLoadEnvOverrides — env перекрывают дефолты.
func TestLoadEnvOverrides(t *testing.T) {
	t.Setenv("HTTP_ADDR", ":9090")
	t.Setenv("PLANNER_MODE", "stub")
	t.Setenv("DGS_CLIENT", "api")
	t.Setenv("LLM_TIMEOUT", "30s")
	t.Setenv("LLM_FORCE_JSON", "false")
	c := Load()
	if c.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if c.PlannerMode != PlannerStub {
		t.Errorf("PlannerMode = %q", c.PlannerMode)
	}
	if c.Dooglys.Mode != DooglysAPI {
		t.Errorf("Dooglys.Mode = %q", c.Dooglys.Mode)
	}
	if c.LLM.Timeout != 30*time.Second {
		t.Errorf("LLM.Timeout = %v", c.LLM.Timeout)
	}
	if c.LLM.ForceJSON {
		t.Error("LLM.ForceJSON must be false")
	}
}

func TestEnvBool(t *testing.T) {
	cases := []struct {
		val  string
		def  bool
		want bool
	}{
		{"1", false, true}, {"true", false, true}, {"TRUE", false, true}, {"yes", false, true},
		{"0", true, false}, {"false", true, false}, {"no", true, false},
		{"", true, true}, {"", false, false}, // пусто → дефолт
		{"garbage", true, true}, // мусор → дефолт
	}
	for _, tc := range cases {
		t.Setenv("X_BOOL", tc.val)
		if got := envBool("X_BOOL", tc.def); got != tc.want {
			t.Errorf("envBool(%q, def=%v) = %v, want %v", tc.val, tc.def, got, tc.want)
		}
	}
}

func TestEnvDuration(t *testing.T) {
	t.Setenv("X_DUR", "2m")
	if got := envDuration("X_DUR", time.Second); got != 2*time.Minute {
		t.Errorf("envDuration = %v, want 2m", got)
	}
	t.Setenv("X_DUR", "garbage")
	if got := envDuration("X_DUR", 5*time.Second); got != 5*time.Second {
		t.Errorf("envDuration(garbage) = %v, want default 5s", got)
	}
}

func TestEnvInt64CSV(t *testing.T) {
	t.Setenv("X_CSV", "123, 456 ,oops,789")
	got := envInt64CSV("X_CSV")
	want := []int64{123, 456, 789} // нечисловой "oops" тихо пропущен
	if len(got) != len(want) {
		t.Fatalf("envInt64CSV = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("envInt64CSV[%d] = %d, want %d", i, got[i], want[i])
		}
	}
	t.Setenv("X_CSV", "")
	if envInt64CSV("X_CSV") != nil {
		t.Error("empty CSV must yield nil (allowlist off)")
	}
}

// TestLoadTenantsLegacy — без TENANTS синтезируется один тенант из TELEGRAM_*/DGS_*.
func TestLoadTenantsLegacy(t *testing.T) {
	t.Setenv("TENANTS", "")
	t.Setenv("TELEGRAM_TENANT", "rukagreka")
	t.Setenv("TELEGRAM_TOKEN", "bot-tok")
	t.Setenv("TELEGRAM_ALLOWLIST", "111,222")
	t.Setenv("DGS_DOMAIN", "rukagreka")
	t.Setenv("DGS_ACCESS_TOKEN", "acc")
	c := Load()
	if len(c.Tenants) != 1 {
		t.Fatalf("Tenants = %d, want 1 (legacy synth)", len(c.Tenants))
	}
	tc := c.Tenants[0]
	if tc.ID != "rukagreka" || tc.BotToken != "bot-tok" || tc.Domain != "rukagreka" || tc.AccessToken != "acc" {
		t.Errorf("synth tenant = %+v", tc)
	}
	if len(tc.Allowlist) != 2 || tc.Allowlist[0] != 111 {
		t.Errorf("allowlist = %v", tc.Allowlist)
	}
}

// TestLoadTenantsIndexed — TENANTS + TENANT_<k>_* даёт список из N тенантов.
func TestLoadTenantsIndexed(t *testing.T) {
	t.Setenv("TENANTS", "a, b")
	t.Setenv("TENANT_a_ID", "rukagreka")
	t.Setenv("TENANT_a_BOT_TOKEN", "tok-a")
	t.Setenv("TENANT_a_ALLOWLIST", "1,2")
	t.Setenv("TENANT_a_ACCESS_TOKEN", "acc-a")
	t.Setenv("TENANT_b_BOT_TOKEN", "tok-b")
	t.Setenv("TENANT_b_DOMAIN", "second")
	c := Load()
	if len(c.Tenants) != 2 {
		t.Fatalf("Tenants = %d, want 2", len(c.Tenants))
	}
	a := c.Tenants[0]
	if a.ID != "rukagreka" || a.Domain != "rukagreka" || a.BotToken != "tok-a" || a.AccessToken != "acc-a" {
		t.Errorf("tenant a = %+v", a)
	}
	b := c.Tenants[1]
	if b.ID != "b" || b.Domain != "second" || b.BotToken != "tok-b" {
		t.Errorf("tenant b = %+v (ID default=key, Domain override)", b)
	}
}

// TestValidate — контракт авторизации падает на старте при битом конфиге (docs/12 §8).
func TestValidate(t *testing.T) {
	base := func() Config {
		return Config{
			Dooglys: Dooglys{Mode: DooglysAPI, ReportAuth: "token"},
			Tenants: []TenantConfig{{ID: "t1", Domain: "t1"}},
		}
	}

	// token без access-token (ни пер-тенантного, ни общего) → ошибка.
	if err := base().Validate(); err == nil {
		t.Error("token без ACCESS_TOKEN должен падать")
	}
	// пер-тенантный access-token → ок.
	c := base()
	c.Tenants[0].AccessToken = "acc"
	if err := c.Validate(); err != nil {
		t.Errorf("пер-тенантный access-token: %v", err)
	}
	// общий DGS_ACCESS_TOKEN → ок.
	c = base()
	c.Dooglys.AccessToken = "shared"
	if err := c.Validate(); err != nil {
		t.Errorf("общий access-token: %v", err)
	}
	// xcontext без x-context → ошибка.
	c = base()
	c.Dooglys.ReportAuth = "xcontext"
	if err := c.Validate(); err == nil {
		t.Error("xcontext без XCONTEXT должен падать")
	}
	// xcontext c x-context → ок.
	c.Tenants[0].XContext = `{"tenant_id":"x"}`
	if err := c.Validate(); err != nil {
		t.Errorf("xcontext c XCONTEXT: %v", err)
	}
	// неизвестный auth → ошибка всегда.
	c = base()
	c.Dooglys.ReportAuth = "weird"
	if err := c.Validate(); err == nil {
		t.Error("неизвестный DGS_REPORT_AUTH должен падать")
	}
	// fixture-режим: креды не нужны → ок.
	c = base()
	c.Dooglys.Mode = DooglysFixture
	if err := c.Validate(); err != nil {
		t.Errorf("fixture-режим не должен требовать креды: %v", err)
	}
}

// TestValidateTelegram — каждый тенант обязан нести токен бота (предпосылка запуска N ботов).
func TestValidateTelegram(t *testing.T) {
	c := Config{Tenants: []TenantConfig{{ID: "t1", BotToken: "x"}, {ID: "t2"}}}
	if err := c.ValidateTelegram(); err == nil {
		t.Error("тенант без токена бота должен падать")
	}
	c.Tenants[1].BotToken = "y"
	if err := c.ValidateTelegram(); err != nil {
		t.Errorf("все токены заданы: %v", err)
	}
}

// TestValidateTelegramProdAllowlist — в проде пустой allowlist на тенанта → fail-fast;
// в dev (и пустой APP_ENV) «пусто = открыт» допустимо.
func TestValidateTelegramProdAllowlist(t *testing.T) {
	// prod + тенант без allowlist → ошибка (бот иначе открыт всем).
	prodOpen := Config{
		AppEnv:  "prod",
		Tenants: []TenantConfig{{ID: "t1", BotToken: "x"}},
	}
	if err := prodOpen.ValidateTelegram(); err == nil {
		t.Error("prod без allowlist должен падать (бот открыт всем)")
	}

	// prod + непустой allowlist → ок. Регистр APP_ENV не важен.
	prodOK := Config{
		AppEnv:  "PROD",
		Tenants: []TenantConfig{{ID: "t1", BotToken: "x", Allowlist: []int64{111}}},
	}
	if err := prodOK.ValidateTelegram(); err != nil {
		t.Errorf("prod с allowlist не должен падать: %v", err)
	}

	// dev + пустой allowlist → ок (прежнее послабление для фикстур/отладки).
	dev := Config{
		AppEnv:  "dev",
		Tenants: []TenantConfig{{ID: "t1", BotToken: "x"}},
	}
	if err := dev.ValidateTelegram(); err != nil {
		t.Errorf("dev без allowlist не должен падать: %v", err)
	}

	// пустой APP_ENV трактуется как небоевой (не prod) → пустой allowlist допустим.
	unset := Config{Tenants: []TenantConfig{{ID: "t1", BotToken: "x"}}}
	if err := unset.ValidateTelegram(); err != nil {
		t.Errorf("пустой APP_ENV не prod → без allowlist ок: %v", err)
	}
}

// TestSummaryNoSecrets — строка для логов не содержит секретов (cookie/пароль/токен).
func TestSummaryNoSecrets(t *testing.T) {
	t.Setenv("DGS_CLIENT", "api")
	t.Setenv("DGS_PASSWORD", "s3cret-pass")
	t.Setenv("DGS_COOKIE", "session=topsecret")
	t.Setenv("AUTH_TOKEN", "demo-token-xyz")
	t.Setenv("TELEGRAM_TOKEN", "tg-bot-token")
	t.Setenv("DGS_ACCESS_TOKEN", "report-access-secret")
	t.Setenv("DGS_XCONTEXT", `{"tenant_id":"xctx-secret"}`)
	s := Load().Summary()
	for _, leak := range []string{"s3cret-pass", "topsecret", "demo-token-xyz", "tg-bot-token", "report-access-secret", "xctx-secret"} {
		if strings.Contains(s, leak) {
			t.Errorf("Summary leaked secret %q: %s", leak, s)
		}
	}
}
