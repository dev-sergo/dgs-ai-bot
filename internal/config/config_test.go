package config

import (
	"strings"
	"testing"
	"time"
)

// TestLoadDefaults — без env Load даёт документированные дефолты под локалку.
func TestLoadDefaults(t *testing.T) {
	// Изолируем от окружения CI: явно гасим то, что может протечь.
	for _, k := range []string{"HTTP_ADDR", "PLANNER_MODE", "DGS_CLIENT", "LLM_FORCE_JSON", "AUTH_TOKEN"} {
		t.Setenv(k, "")
	}
	c := Load()
	if c.HTTPAddr != ":8088" {
		t.Errorf("HTTPAddr = %q, want :8088", c.HTTPAddr)
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
