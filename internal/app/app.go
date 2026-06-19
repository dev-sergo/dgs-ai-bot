// Package app — оркестратор пайплайна: planner → validate → dates → client → engine → render.
// Не зависит от HTTP, поэтому полностью покрывается интеграционными тестами.
package app

import (
	"context"
	"time"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dates"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/engine"
	"dgsbot/internal/envelope"
	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
	"dgsbot/internal/render"
	"dgsbot/internal/tenantctx"
)

// Answer — результат обработки запроса.
type Answer struct {
	TenantID   string                `json:"tenant_id"`
	Plan       plan.AnalysisPlan     `json:"plan"`
	Validation plan.ValidationResult `json:"validation"`
	Envelope   *envelope.Envelope    `json:"envelope,omitempty"`
	Text       string                `json:"answer,omitempty"`
}

// App — собранный пайплайн.
type App struct {
	planner planner.Planner
	tenants *tenantctx.Store
	client  dooglys.Client
	cat     *catalog.Catalog

	// Now инъектируется для детерминированных тестов.
	Now func() time.Time
}

// New собирает оркестратор.
func New(pl planner.Planner, tenants *tenantctx.Store, client dooglys.Client) *App {
	return &App{
		planner: pl,
		tenants: tenants,
		client:  client,
		cat:     catalog.Default(),
		Now:     time.Now,
	}
}

// Ask — основной вход: текст → ответ.
func (a *App) Ask(ctx context.Context, tenantID, text string) (Answer, error) {
	ans := Answer{TenantID: tenantID}

	p, err := a.planner.Plan(ctx, text)
	if err != nil {
		return ans, err
	}
	ans.Plan = p

	ans.Validation = plan.Validate(&p, a.cat)
	if !ans.Validation.OK {
		// Невалидно или нужно уточнение — отдаём как есть (ветка clarify/refusal).
		if ans.Validation.NeedClarify {
			ans.Text = ans.Validation.ClarifyPrompt
		}
		return ans, nil
	}

	// Контекст тенанта (таймзона/валюта). Неизвестный → дефолт Москва/RUB.
	t, ok := a.tenants.Lookup(tenantID)
	if !ok {
		t = tenantctx.Tenant{Timezone: "Europe/Moscow", Currency: "RUB", CurrencyPrecision: 2}
	}

	// Резолв периода в абсолютные даты по таймзоне тенанта.
	from, to, err := a.resolvePeriod(p.Period, t)
	if err != nil {
		return ans, err
	}

	res, err := a.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: from, To: to})
	if err != nil {
		return ans, err
	}

	rep, _ := a.cat.Report(p.Report)
	period := envelope.Period{From: from, To: to, TZ: t.Timezone}
	env := engine.Plain(p, rep, res, tenantID, currencyOr(t.Currency), period)
	ans.Envelope = &env
	ans.Text = render.Text(env)
	return ans, nil
}

func (a *App) resolvePeriod(p plan.Period, t tenantctx.Tenant) (from, to string, err error) {
	if p.Kind == "explicit" {
		return p.From, p.To, nil
	}
	r, err := dates.Resolve(p.Token, t.Location(), a.Now())
	if err != nil {
		return "", "", err
	}
	return r.From, r.To, nil
}

func currencyOr(c string) string {
	if c == "" {
		return "RUB"
	}
	return c
}
