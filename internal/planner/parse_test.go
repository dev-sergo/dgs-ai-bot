package planner

import "testing"

func TestParsePlan_Single(t *testing.T) {
	raw := `{"version":"1","intent":"report","report":"payment","period":{"kind":"relative","token":"last_30_days"},"method":"plain"}`
	p, err := parsePlan(raw)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if p.Report != "payment" {
		t.Errorf("report=%q, ожидался payment", p.Report)
	}
	if p.Period.Token != "last_30_days" {
		t.Errorf("period.token=%q, ожидался last_30_days", p.Period.Token)
	}
}

// Два JSON-объекта подряд (составной запрос: модель вернула два отчёта).
// parsePlan должен взять первый и не упасть.
func TestParsePlan_Double(t *testing.T) {
	raw := `{
  "version":"1","intent":"report","report":"payment",
  "period":{"kind":"relative","token":"last_30_days"},"method":"plain"
}
{
  "version":"1","intent":"report","report":"products",
  "period":{"kind":"relative","token":"last_30_days"},"method":"plain"
}`
	p, err := parsePlan(raw)
	if err != nil {
		t.Fatalf("двойной ответ не должен вызывать ошибку: %v", err)
	}
	if p.Intent != "report" {
		t.Errorf("intent=%q, ожидался report", p.Intent)
	}
	if p.Report != "payment" {
		t.Errorf("report=%q, ожидался payment (первый объект)", p.Report)
	}
}

// Текстовый префикс перед JSON (модель добавила пояснение).
func TestParsePlan_TextPrefix(t *testing.T) {
	raw := "Вот план:\n" + `{"version":"1","intent":"report","report":"paycheck","period":{"kind":"relative","token":"today"},"method":"plain"}`
	p, err := parsePlan(raw)
	if err != nil {
		t.Fatalf("текстовый префикс не должен ломать разбор: %v", err)
	}
	if p.Report != "paycheck" {
		t.Errorf("report=%q, ожидался paycheck", p.Report)
	}
}

// Совсем кривой ответ — ошибка, а не паника.
func TestParsePlan_Invalid(t *testing.T) {
	_, err := parsePlan("это не JSON вообще")
	if err == nil {
		t.Fatal("ожидалась ошибка на невалидном вводе")
	}
}

// Version по умолчанию проставляется, если отсутствует в ответе модели.
func TestParsePlan_DefaultVersion(t *testing.T) {
	raw := `{"intent":"report","report":"orders","period":{"kind":"relative","token":"today"},"method":"plain"}`
	p, err := parsePlan(raw)
	if err != nil {
		t.Fatalf("неожиданная ошибка: %v", err)
	}
	if p.Version != "1" {
		t.Errorf("version=%q, ожидался \"1\"", p.Version)
	}
}
