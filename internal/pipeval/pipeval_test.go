package pipeval

import (
	"strings"
	"testing"

	"dgsbot/internal/app"
)

func ptrBool(b bool) *bool { return &b }

// TestCheckContainsAny — хотя бы одна подстрока должна присутствовать; ни одной → расхождение.
func TestCheckContainsAny(t *testing.T) {
	ans := app.Answer{Text: "Выручка за период: 1000 руб. На чём уходят деньги: скидки — 50 руб."}

	if m := Check(ans, Expect{ContainsAny: []string{"возврат", "скидк"}}); len(m) != 0 {
		t.Fatalf("ожидалось совпадение (есть «скидк»), получили расхождения: %v", m)
	}
	m := Check(ans, Expect{ContainsAny: []string{"возврат", "налог"}})
	if len(m) != 1 || !strings.Contains(m[0], "ни одной") {
		t.Fatalf("ожидалось одно расхождение про «ни одной», получили: %v", m)
	}
}

// TestCheckMentionsNumber — детект цифры в тексте (совет подкреплён числом).
func TestCheckMentionsNumber(t *testing.T) {
	withNum := app.Answer{Text: "Выручка за период: 1000 руб."}
	noNum := app.Answer{Text: "Совет: пересмотрите ассортимент и работайте над качеством."}

	if m := Check(withNum, Expect{MentionsNum: ptrBool(true)}); len(m) != 0 {
		t.Fatalf("ожидалось число найдено, получили расхождения: %v", m)
	}
	if m := Check(noNum, Expect{MentionsNum: ptrBool(true)}); len(m) != 1 {
		t.Fatalf("ожидалось расхождение (числа нет), получили: %v", m)
	}
	if m := Check(noNum, Expect{MentionsNum: ptrBool(false)}); len(m) != 0 {
		t.Fatalf("ожидалось mentions_number=false без числа — без расхождений, получили: %v", m)
	}
}
