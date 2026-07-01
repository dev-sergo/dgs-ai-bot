package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"dgsbot/internal/dooglys"
	"dgsbot/internal/resolver"
	"dgsbot/internal/tenantctx"
)

// errClient — источник, всегда падающий на Fetch (симуляция флака/таймаута живого API).
type errClient struct{ err error }

func (c *errClient) Fetch(_ context.Context, _ dooglys.Query) (dooglys.Result, error) {
	return dooglys.Result{}, c.err
}

// TestGraceful_FetchErrorSoftAnswer: сбой client.Fetch превращается в человеческий ответ
// (err=nil, мягкий текст), а НЕ уходит наружу как raw error/500. Так и Telegram, и HTTP
// деградируют мягко, а реальная ошибка остаётся в логах.
func TestGraceful_FetchErrorSoftAnswer(t *testing.T) {
	a := NewMulti(nil, &tenantctx.Store{}, nil, nil, nil)
	a.Now = func() time.Time { return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) }
	a.Register("tenant-A", &errClient{err: errors.New("dial tcp: connection refused")}, &resolver.Store{})

	ans, err := a.executeReport(context.Background(), "tenant-A", "s1", "выручка за июнь", paymentPlan())
	if err != nil {
		t.Fatalf("сбой Fetch не должен всплывать как ошибка наружу, got err=%v", err)
	}
	if ans.Text != fetchErrorPrompt {
		t.Errorf("ожидали мягкий текст %q, got %q", fetchErrorPrompt, ans.Text)
	}
	if ans.Validation.OK {
		t.Error("при сбое данных Validation.OK должен быть false")
	}
	if ans.Envelope != nil {
		t.Error("при сбое данных не должно быть envelope (нечего рендерить)")
	}
}

// TestGraceful_UnknownTenantStillErrors: транзиентный сбой данных деградирует мягко, но
// инвариант конфига (незарегистрированный тенант) по-прежнему возвращает ошибку —
// это разные классы (изоляция не должна тихо превращаться в «попробуйте позже»).
func TestGraceful_UnknownTenantStillErrors(t *testing.T) {
	a := NewMulti(nil, &tenantctx.Store{}, nil, nil, nil)
	a.Now = func() time.Time { return time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC) }

	_, err := a.executeReport(context.Background(), "tenant-Z", "s1", "выручка", paymentPlan())
	if !errors.Is(err, errUnknownTenant) {
		t.Fatalf("незарегистрированный тенант должен вернуть errUnknownTenant, got %v", err)
	}
}
