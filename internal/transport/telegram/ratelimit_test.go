package telegram

import (
	"testing"
	"time"
)

// clock — управляемое время для детерминированной проверки окна лимитера.
type clock struct{ t time.Time }

func (c *clock) now() time.Time      { return c.t }
func (c *clock) add(d time.Duration) { c.t = c.t.Add(d) }

// TestRateLimiter_PassesUpToLimit: первые N запросов проходят, N+1 отбит.
func TestRateLimiter_PassesUpToLimit(t *testing.T) {
	rl := newRateLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if !rl.allow(42) {
			t.Fatalf("запрос %d в пределах лимита должен пройти", i+1)
		}
	}
	if rl.allow(42) {
		t.Error("запрос сверх лимита (N+1) должен быть отбит")
	}
}

// TestRateLimiter_WindowResets: после сдвига времени за окно счётчик сбрасывается.
func TestRateLimiter_WindowResets(t *testing.T) {
	clk := &clock{t: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)}
	rl := newRateLimiter(2, time.Minute)
	rl.now = clk.now

	if !rl.allow(1) || !rl.allow(1) {
		t.Fatal("первые 2 запроса должны пройти")
	}
	if rl.allow(1) {
		t.Fatal("3-й запрос в том же окне должен быть отбит")
	}
	clk.add(61 * time.Second) // окно истекло
	if !rl.allow(1) {
		t.Error("после сброса окна запрос снова должен проходить")
	}
}

// TestRateLimiter_PerChatIndependent: лимит одного чата не влияет на другой.
func TestRateLimiter_PerChatIndependent(t *testing.T) {
	rl := newRateLimiter(1, time.Minute)
	if !rl.allow(1) {
		t.Fatal("первый запрос чата 1 должен пройти")
	}
	if rl.allow(1) {
		t.Fatal("второй запрос чата 1 должен быть отбит")
	}
	if !rl.allow(2) {
		t.Error("чат 2 не должен зависеть от лимита чата 1")
	}
}

// TestRateLimiter_SpamDoesNotExtendWindow: непрерывный спам не продлевает окно —
// отбитые попытки не пишутся, поэтому по истечении окна лимит открывается.
func TestRateLimiter_SpamDoesNotExtendWindow(t *testing.T) {
	clk := &clock{t: time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)}
	rl := newRateLimiter(1, time.Minute)
	rl.now = clk.now

	if !rl.allow(7) {
		t.Fatal("первый запрос должен пройти")
	}
	// Спамим внутри окна — все отбиты и НЕ продлевают его.
	for i := 0; i < 5; i++ {
		clk.add(10 * time.Second)
		if rl.allow(7) {
			t.Fatalf("спам-попытка %d внутри окна должна быть отбита", i+1)
		}
	}
	// С момента первого (и единственного зачтённого) запроса прошло >60с.
	clk.add(20 * time.Second)
	if !rl.allow(7) {
		t.Error("окно должно отсчитываться от зачтённого запроса, а не от последнего спама")
	}
}

// TestRateLimiter_DisabledPassesAll: limit<=0 или nil-приёмник → лимитер выключен.
func TestRateLimiter_DisabledPassesAll(t *testing.T) {
	var rl *rateLimiter // nil
	for i := 0; i < 100; i++ {
		if !rl.allow(1) {
			t.Fatal("nil-лимитер должен пропускать всё")
		}
	}
	off := newRateLimiter(0, time.Minute)
	for i := 0; i < 100; i++ {
		if !off.allow(1) {
			t.Fatal("выключенный лимитер (limit=0) должен пропускать всё")
		}
	}
}
