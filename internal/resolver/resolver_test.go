package resolver

import (
	"sync"
	"testing"

	"dgsbot/internal/dooglys"
)

const fixturesDir = "../../docs/contracts/fixtures"

func store(t *testing.T) *Store {
	t.Helper()
	return Load(fixturesDir)
}

func TestResolveSalePointUnique(t *testing.T) {
	m, err := store(t).Resolve("sale_point", "Казанский вокзал")
	if err != nil {
		t.Fatalf("не резолвится уникальная точка: %v", err)
	}
	if m.UUID == "" {
		t.Error("ожидался непустой uuid")
	}
}

func TestResolveLocalityByShortName(t *testing.T) {
	// У региона есть краткое имя «НН» (вторая колонка).
	m, err := store(t).Resolve("locality", "НН")
	if err != nil {
		t.Fatalf("не резолвится регион по краткому имени: %v", err)
	}
	if m.UUID == "" {
		t.Error("ожидался непустой uuid региона")
	}
}

func TestResolveNotFound(t *testing.T) {
	_, err := store(t).Resolve("sale_point", "Точки-такой-нет-12345")
	re, ok := err.(*ResolveError)
	if !ok || re.Ambiguous {
		t.Fatalf("ожидалась ошибка not-found, получено: %v", err)
	}
}

func TestResolveAmbiguous(t *testing.T) {
	// «Авто» встречается в нескольких точках (АвтоPizza, Автосуши №1).
	_, err := store(t).Resolve("sale_point", "Авто")
	re, ok := err.(*ResolveError)
	if !ok || !re.Ambiguous {
		t.Fatalf("ожидалась неоднозначность, получено: %v", err)
	}
	if len(re.Candidates) < 2 {
		t.Errorf("ожидалось ≥2 кандидата, получено: %v", re.Candidates)
	}
}

// TestSetOptionsThenResolve — фоновый индекс товаров (api-режим) приходит через
// SetOptions уже после старта; после него Resolve должен видеть живой товар.
func TestSetOptionsThenResolve(t *testing.T) {
	s := store(t)
	s.SetOptions("product", []dooglys.SelectOption{{UUID: "u-1", Name: "Раф апельсиновый"}})
	m, err := s.Resolve("product", "Раф апельсиновый")
	if err != nil {
		t.Fatalf("живой товар не резолвится после SetOptions: %v", err)
	}
	if m.UUID != "u-1" {
		t.Errorf("ожидался uuid u-1, получено %q", m.UUID)
	}
}

// TestConcurrentSetOptionsAndResolve — SetOptions из фоновой горутины конкурентно с
// Resolve обслуживающего потока не должны паниковать на гонке карты (Store под RWMutex).
// Запускать с -race для реальной проверки; здесь — дымовой прогон, что код не падает.
func TestConcurrentSetOptionsAndResolve(t *testing.T) {
	s := store(t)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			s.SetOptions("product", []dooglys.SelectOption{{UUID: "u-1", Name: "Американо"}})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_, _ = s.Resolve("product", "Американо")
		}
	}()
	wg.Wait()
}
