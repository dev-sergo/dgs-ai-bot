package app

import (
	"context"
	"strings"
	"testing"

	"dgsbot/internal/session"
	"dgsbot/internal/tenantctx"
)

// TestAsk_DisabledTenant — kill-switch (TENANT_<k>_ENABLED=0): выключенный тенант
// получает maintenancePrompt ДО планирования и доступа к данным (planner=nil не
// вызывается — иначе тест бы паниковал), остальные тенанты не затронуты.
func TestAsk_DisabledTenant(t *testing.T) {
	a := NewMulti(nil, &tenantctx.Store{}, nil, nil, session.NewStore())
	a.Disable("tenant-A")

	ans, err := a.Ask(context.Background(), "tenant-A", "s1", "выручка за вчера")
	if err != nil {
		t.Fatalf("kill-switch должен отвечать мягко, got err=%v", err)
	}
	if ans.Text != maintenancePrompt {
		t.Errorf("ожидали %q, got %q", maintenancePrompt, ans.Text)
	}
}

// TestAsk_QueryTooLong — «простыня» отбивается ДО планировщика мягким текстом
// (planner=nil не вызывается), а не уходит в промпт модели или как 500.
func TestAsk_QueryTooLong(t *testing.T) {
	a := NewMulti(nil, &tenantctx.Store{}, nil, nil, session.NewStore())

	long := strings.Repeat("а", maxQueryRunes+1)
	ans, err := a.Ask(context.Background(), "tenant-A", "s1", long)
	if err != nil {
		t.Fatalf("сверхдлинный запрос должен отбиваться мягко, got err=%v", err)
	}
	if ans.Text != queryTooLongPrompt {
		t.Errorf("ожидали %q, got %q", queryTooLongPrompt, ans.Text)
	}
}

// TestAsk_QueryAtLimitPasses — реплика ровно в лимит НЕ отбивается гейтом длины
// (доходит до обычного пайплайна; с nil-планировщиком это ошибка/паника дальше по
// пути — здесь важно лишь, что текст ответа НЕ queryTooLongPrompt).
func TestAsk_QueryAtLimitPasses(t *testing.T) {
	a := NewMulti(nil, &tenantctx.Store{}, nil, nil, session.NewStore())
	a.Disable("tenant-A") // перехват дальше по пайплайну, чтобы не дёргать nil-планировщик

	ans, err := a.Ask(context.Background(), "tenant-A", "s1", strings.Repeat("а", maxQueryRunes))
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ans.Text == queryTooLongPrompt {
		t.Error("реплика ровно в лимит не должна отбиваться гейтом длины")
	}
}
