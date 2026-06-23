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

	if m := Check(ans, Expect{ContainsAny: AnyGroups{{"возврат", "скидк"}}}); len(m) != 0 {
		t.Fatalf("ожидалось совпадение (есть «скидк»), получили расхождения: %v", m)
	}
	m := Check(ans, Expect{ContainsAny: AnyGroups{{"возврат", "налог"}}})
	if len(m) != 1 || !strings.Contains(m[0], "ни одной") {
		t.Fatalf("ожидалось одно расхождение про «ни одной», получили: %v", m)
	}
}

// TestCheckContainsAnyGroups — несколько OR-групп: каждая группа должна дать хотя бы одно совпадение.
func TestCheckContainsAnyGroups(t *testing.T) {
	ans := app.Answer{Text: "Возвраты — 200 ₽ (13% выручки), скидки — 105 ₽."}

	// Обе группы выполнены: есть «возврат» и есть «выручк».
	if m := Check(ans, Expect{ContainsAny: AnyGroups{{"возврат", "скидк"}, {"доля", "выручк"}}}); len(m) != 0 {
		t.Fatalf("обе группы должны совпасть, получили расхождения: %v", m)
	}
	// Вторая группа не выполнена (нет относительной метрики) → ровно одно расхождение.
	noRel := app.Answer{Text: "Возвраты — 200 ₽, скидки — 105 ₽."}
	m := Check(noRel, Expect{ContainsAny: AnyGroups{{"возврат", "скидк"}, {"доля", "выручк"}}})
	if len(m) != 1 || !strings.Contains(m[0], "ни одной") {
		t.Fatalf("ожидалось одно расхождение по второй группе, получили: %v", m)
	}
}

// TestAnyGroupsUnmarshal — JSON принимает и плоский, и вложенный формат.
func TestAnyGroupsUnmarshal(t *testing.T) {
	var flat AnyGroups
	if err := flat.UnmarshalJSON([]byte(`["a","b"]`)); err != nil {
		t.Fatalf("плоский формат: %v", err)
	}
	if len(flat) != 1 || len(flat[0]) != 2 {
		t.Fatalf("плоский ["+`"a","b"`+"] → одна группа из 2, got %v", flat)
	}
	var nested AnyGroups
	if err := nested.UnmarshalJSON([]byte(`[["a","b"],["c"]]`)); err != nil {
		t.Fatalf("вложенный формат: %v", err)
	}
	if len(nested) != 2 || len(nested[0]) != 2 || len(nested[1]) != 1 {
		t.Fatalf("вложенный → 2 группы, got %v", nested)
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
