// Package config — конфигурация сервиса из переменных окружения (с дефолтами).
package config

import (
	"fmt"
	"os"
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

// Dooglys — настройки HTTP-клиента Dooglys (используется при DooglysMode=http).
type Dooglys struct {
	Mode   DooglysMode
	Base   string // URL тенанта, напр. https://google.dooglys.com
	Cookie string // полное значение заголовка Cookie (из DGS_COOKIE)
}

// Config — корневая конфигурация.
type Config struct {
	HTTPAddr     string
	PlannerMode  PlannerMode
	FixturesPath string
	LLM          LLM
	Dooglys      Dooglys
}

// Load читает конфиг из ENV с разумными дефолтами под локальную разработку.
func Load() Config {
	return Config{
		HTTPAddr:     env("HTTP_ADDR", ":8088"),
		PlannerMode:  PlannerMode(env("PLANNER_MODE", string(PlannerLLM))),
		FixturesPath: env("FIXTURES_PATH", "docs/contracts/fixtures"),
		LLM: LLM{
			BaseURL:   env("LLM_BASE_URL", "http://172.20.10.2:8080"),
			Model:     env("LLM_MODEL", "qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07"),
			APIKey:    env("LLM_API_KEY", ""),
			Timeout:   envDuration("LLM_TIMEOUT", 180*time.Second),
			ForceJSON: envBool("LLM_FORCE_JSON", true),
		},
		Dooglys: Dooglys{
			Mode:   DooglysMode(env("DGS_CLIENT", string(DooglysFixture))),
			Base:   env("DGS_BASE", "https://google.dooglys.com"),
			Cookie: env("DGS_COOKIE", ""),
		},
	}
}

// Summary — однострочное описание для логов (без секретов).
func (c Config) Summary() string {
	dgs := string(c.Dooglys.Mode)
	if c.Dooglys.Mode == DooglysHTTP {
		dgs = "http(" + c.Dooglys.Base + ")"
	}
	return fmt.Sprintf("addr=%s planner=%s llm=%s model=%s fixtures=%s dooglys=%s",
		c.HTTPAddr, c.PlannerMode, c.LLM.BaseURL, c.LLM.Model, c.FixturesPath, dgs)
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
