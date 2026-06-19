// Package app — оркестратор пайплайна: planner → validate → dates → client → engine → render.
// Не зависит от HTTP, поэтому полностью покрывается интеграционными тестами.
package app

import (
	"context"
	"strings"
	"time"

	"dgsbot/internal/catalog"
	"dgsbot/internal/dates"
	"dgsbot/internal/dooglys"
	"dgsbot/internal/engine"
	"dgsbot/internal/envelope"
	"dgsbot/internal/narrator"
	"dgsbot/internal/plan"
	"dgsbot/internal/planner"
	"dgsbot/internal/render"
	"dgsbot/internal/resolver"
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
	planner  planner.Planner
	tenants  *tenantctx.Store
	client   dooglys.Client
	resolver *resolver.Store
	narrator narrator.Narrator
	cat      *catalog.Catalog

	// Now инъектируется для детерминированных тестов.
	Now func() time.Time
}

// New собирает оркестратор.
func New(pl planner.Planner, tenants *tenantctx.Store, client dooglys.Client, res *resolver.Store, nar narrator.Narrator) *App {
	return &App{
		planner:  pl,
		tenants:  tenants,
		client:   client,
		resolver: res,
		narrator: nar,
		cat:      catalog.Default(),
		Now:      time.Now,
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

	rep, _ := a.cat.Report(p.Report)

	// Резолв фильтров: имена → uuid (для ref). Нерезолвнутое имя → уточнение.
	filters, clarify := a.resolveFilters(rep, p.Filters)
	if clarify != "" {
		ans.Validation.OK = false
		ans.Validation.NeedClarify = true
		ans.Validation.ClarifyPrompt = clarify
		ans.Text = clarify
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

	period := envelope.Period{From: from, To: to, TZ: t.Timezone}
	currency := currencyOr(t.Currency)
	resNow, err := a.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: from, To: to, Filters: filters})
	if err != nil {
		return ans, err
	}

	var env envelope.Envelope
	switch p.Method {
	case "compare", "contribution":
		prevR, err := a.comparePeriod(p, dates.Range{From: from, To: to})
		if err != nil {
			return ans, err
		}
		periodPrev := envelope.Period{From: prevR.From, To: prevR.To, TZ: t.Timezone}
		resPrev, err := a.client.Fetch(ctx, dooglys.Query{Report: p.Report, From: prevR.From, To: prevR.To, Filters: filters})
		if err != nil {
			return ans, err
		}
		metric := primaryMetric(p)
		if p.Method == "compare" {
			env = engine.Compare(rep, metric, resNow, resPrev, tenantID, currency, period, periodPrev)
		} else {
			env = engine.Contribution(rep, metric, resNow, resPrev, p.TopN, tenantID, currency, period, periodPrev)
		}
		if a.narrator != nil {
			if txt, nerr := a.narrator.Narrate(ctx, env); nerr == nil && txt != "" {
				env.Narrative = txt
			}
		}
	default:
		env = engine.Plain(p, rep, resNow, tenantID, currency, period)
	}

	if len(resNow.FiltersApplied) > 0 {
		env.Meta["filters_applied"] = resNow.FiltersApplied
	}
	if len(resNow.FiltersSkipped) > 0 {
		env.Meta["filters_skipped"] = resNow.FiltersSkipped
	}
	ans.Envelope = &env
	ans.Text = render.Text(env)
	return ans, nil
}

// resolveFilters превращает фильтры плана в фильтры запроса.
// Для ref-фильтров имена резолвятся в uuid; первая неудача → текст уточнения.
func (a *App) resolveFilters(rep catalog.Report, pfs []plan.Filter) ([]dooglys.QueryFilter, string) {
	var out []dooglys.QueryFilter
	for _, pf := range pfs {
		cf, ok := rep.FilterByField(pf.Field)
		if !ok {
			continue // валидатор уже отсёк неизвестные; страхуемся
		}
		qf := dooglys.QueryFilter{Field: pf.Field, Param: cf.Param, Names: pf.Values}
		if cf.Kind == "ref" {
			for _, name := range pf.Values {
				m, err := a.resolver.Resolve(pf.Field, name)
				if err != nil {
					if re, ok := err.(*resolver.ResolveError); ok && re.Ambiguous {
						return nil, "Уточните " + pf.Field + " «" + name + "»: подходят " + joinRu(re.Candidates) + "."
					}
					return nil, "Не нашёл " + pf.Field + " «" + name + "». Проверьте название."
				}
				qf.UUIDs = append(qf.UUIDs, m.UUID)
			}
		}
		out = append(out, qf)
	}
	return out, ""
}

func joinRu(ss []string) string {
	return strings.Join(ss, ", ")
}

// comparePeriod определяет период сравнения: явный из плана или предыдущий равный.
func (a *App) comparePeriod(p plan.AnalysisPlan, cur dates.Range) (dates.Range, error) {
	if p.CompareTo != nil && p.CompareTo.Kind == "explicit" && p.CompareTo.From != "" {
		return dates.Range{From: p.CompareTo.From, To: p.CompareTo.To}, nil
	}
	return dates.PrevRange(cur)
}

func primaryMetric(p plan.AnalysisPlan) string {
	if len(p.Metrics) > 0 {
		return p.Metrics[0]
	}
	return "sum_all"
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
