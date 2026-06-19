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

// LLM — настройки OpenAI-совместимого эндпоинта (llama.cpp на риге).
type LLM struct {
	BaseURL string
	Model   string
	APIKey  string
	Timeout time.Duration
}

// Config — корневая конфигурация.
type Config struct {
	HTTPAddr     string
	PlannerMode  PlannerMode
	FixturesPath string
	LLM          LLM
}

// Load читает конфиг из ENV с разумными дефолтами под локальную разработку.
func Load() Config {
	return Config{
		HTTPAddr:     env("HTTP_ADDR", ":8088"),
		PlannerMode:  PlannerMode(env("PLANNER_MODE", string(PlannerLLM))),
		FixturesPath: env("FIXTURES_PATH", "docs/contracts/fixtures"),
		LLM: LLM{
			BaseURL: env("LLM_BASE_URL", "http://172.20.10.2:8080"),
			Model:   env("LLM_MODEL", "qwen2-5-32b-instruct-q4-k-m-ctx-8k-q8-0-kv-t07"),
			APIKey:  env("LLM_API_KEY", ""),
			Timeout: envDuration("LLM_TIMEOUT", 60*time.Second),
		},
	}
}

// Summary — однострочное описание для логов (без секретов).
func (c Config) Summary() string {
	return fmt.Sprintf("addr=%s planner=%s llm=%s model=%s fixtures=%s",
		c.HTTPAddr, c.PlannerMode, c.LLM.BaseURL, c.LLM.Model, c.FixturesPath)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
