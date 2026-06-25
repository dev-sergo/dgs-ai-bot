package plan

import (
	"testing"

	"dgsbot/internal/catalog"
)

func base() AnalysisPlan {
	return AnalysisPlan{
		Version: "1", Class: ClassA, Report: "payment",
		Metrics: []string{"sum_all"}, GroupBy: []string{"date"},
		Period: Period{Kind: "relative", Token: "today"},
		Method: "plain", Output: Output{Format: "text"},
	}
}

func TestValidateOK(t *testing.T) {
	p := base()
	res := Validate(&p, catalog.Default())
	if !res.OK {
		t.Fatalf("ожидался OK, получено: %+v", res)
	}
}

func TestValidateUnknownReport(t *testing.T) {
	p := base()
	p.Report = "secret_report"
	res := Validate(&p, catalog.Default())
	if res.OK || len(res.Errors) == 0 {
		t.Fatalf("ожидалась ошибка white-list, получено: %+v", res)
	}
}

func TestValidateRejectsPIIMetric(t *testing.T) {
	p := base()
	p.Report = "paycheck"
	p.Metrics = []string{"cashier_name"} // PII
	res := Validate(&p, catalog.Default())
	if res.OK {
		t.Fatalf("PII-метрика не должна проходить валидацию: %+v", res)
	}
}

func TestValidateBadEnum(t *testing.T) {
	p := base()
	p.Report = "paycheck"
	p.Metrics = []string{"paid"}
	p.Filters = []Filter{{Field: "payment_type", Values: []string{"bitcoin"}}}
	res := Validate(&p, catalog.Default())
	if res.OK {
		t.Fatalf("недопустимое enum-значение должно отклоняться: %+v", res)
	}
}

func TestValidateNeedClarifyWhenNoPeriod(t *testing.T) {
	p := base()
	p.Period = Period{Kind: "relative", Token: ""}
	res := Validate(&p, catalog.Default())
	if res.OK || !res.NeedClarify || res.ClarifyPrompt == "" {
		t.Fatalf("ожидался запрос уточнения периода: %+v", res)
	}
}

func TestValidateUnknownFilter(t *testing.T) {
	p := base()
	p.Filters = []Filter{{Field: "secret_field", Values: []string{"x"}}}
	res := Validate(&p, catalog.Default())
	if res.OK {
		t.Fatalf("неизвестный фильтр должен отклоняться: %+v", res)
	}
}

// ref-фильтр с пустыми values — LLM иногда выдаёт values:[""].
// Contains(n,"") матчит всё → такой фильтр пропускал бы весь справочник.
func TestValidateRefFilterEmptyValues(t *testing.T) {
	p := base()
	p.Report = "payment"
	p.Filters = []Filter{{Field: "locality", Values: []string{"", "  "}}}
	res := Validate(&p, catalog.Default())
	if res.OK {
		t.Fatalf("ref-фильтр с пустыми values должен отклоняться: %+v", res)
	}
}

// Пустая строка в metrics — LLM-мусор, не должна проходить валидацию.
func TestValidateEmptyMetricString(t *testing.T) {
	p := base()
	p.Metrics = []string{""}
	res := Validate(&p, catalog.Default())
	if res.OK {
		t.Fatalf("пустая строка в metrics должна отклоняться: %+v", res)
	}
}
