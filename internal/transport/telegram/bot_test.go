package telegram

import "testing"

// botWith строит Bot без реального Telegram API — для проверки чистых решений
// (allowlist/resolveTenant), не требующих сети.
func botWith(tenantID string, allow ...int64) *Bot {
	al := make(map[int64]struct{}, len(allow))
	for _, id := range allow {
		al[id] = struct{}{}
	}
	return &Bot{tenantID: tenantID, allowlist: al}
}

// TestAllowed_WhitelistRejectsStranger: чужой chat_id отбит; свой пропущен.
func TestAllowed_WhitelistRejectsStranger(t *testing.T) {
	b := botWith("tenant-A", 111, 222)
	if !b.allowed(111) {
		t.Error("111 в whitelist — должен быть пропущен")
	}
	if b.allowed(999) {
		t.Error("999 не в whitelist — должен быть отбит")
	}
}

// TestAllowed_EmptyWhitelistOpen: пустой whitelist → открыт всем (dev/legacy).
func TestAllowed_EmptyWhitelistOpen(t *testing.T) {
	b := botWith("tenant-A")
	if !b.allowed(12345) {
		t.Error("пустой whitelist должен пропускать любой chat_id")
	}
}

// TestResolveTenant_BoundToBot: бот жёстко на своём тенанте — chatID не влияет.
// tenantID приходит из привязки бота, а не из ввода пользователя (изоляция).
func TestResolveTenant_BoundToBot(t *testing.T) {
	b := botWith("tenant-A", 111)
	for _, chatID := range []int64{111, 222, -1} {
		if got := b.resolveTenant(chatID); got != "tenant-A" {
			t.Errorf("resolveTenant(%d) = %q, want tenant-A", chatID, got)
		}
	}
}

// TestWhitelistPerTenant: у каждого бота свой whitelist — разрешённый на боте A
// не автоматически разрешён на боте B (изоляция доступа между тенантами).
func TestWhitelistPerTenant(t *testing.T) {
	botA := botWith("tenant-A", 111)
	botB := botWith("tenant-B", 222)
	if botA.allowed(222) {
		t.Error("222 разрешён только на боте B, не на A")
	}
	if botB.allowed(111) {
		t.Error("111 разрешён только на боте A, не на B")
	}
}
