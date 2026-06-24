package narrator

import (
	"strings"
	"testing"

	"dgsbot/internal/envelope"
)

// Compose поправляет ложную посылку вопроса: «почему упала» (premise=down) при выросших
// числах (delta_abs > 0) начинается с явного уточнения, а не подыгрывает спаду. И наоборот.
func TestComposePremiseCorrection(t *testing.T) {
	base := func(deltaAbs float64, premise string) envelope.Envelope {
		return envelope.Envelope{
			Type:     "payment",
			Currency: "RUB",
			Summary: map[string]float64{
				"value_now": 120, "value_prev": 100,
				"delta_abs": deltaAbs, "delta_pct": deltaAbs / 100 * 100,
			},
			Meta: map[string]any{"premise_dir": premise},
		}
	}

	// premise=down, но выручка выросла → поправка «не падение, а рост».
	if got := Compose(base(20, "down")); !strings.HasPrefix(got, "Уточнение: за период не падение, а рост.") {
		t.Errorf("down при росте: нет поправки посылки, got %q", got)
	}

	// premise=up, но выручка снизилась → поправка «не рост, а снижение».
	down := base(-20, "up")
	down.Summary["value_now"] = 80
	if got := Compose(down); !strings.HasPrefix(got, "Уточнение: за период не рост, а снижение.") {
		t.Errorf("up при спаде: нет поправки посылки, got %q", got)
	}

	// premise совпадает с числами (down + спад) → без поправки.
	ok := base(-20, "down")
	ok.Summary["value_now"] = 80
	if got := Compose(ok); strings.Contains(got, "Уточнение:") {
		t.Errorf("совпадающая посылка не должна давать поправку, got %q", got)
	}

	// premise пусто → без поправки.
	if got := Compose(base(20, "")); strings.Contains(got, "Уточнение:") {
		t.Errorf("пустая посылка не должна давать поправку, got %q", got)
	}
}
