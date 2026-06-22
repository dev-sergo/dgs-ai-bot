package resolver

import (
	"context"
	"testing"

	"dgsbot/internal/dooglys"
)

// fakeSelects реализует SelectSource из заданных в тесте данных (без сети).
type fakeSelects struct {
	data map[string]map[string][]dooglys.SelectOption // report → param → options
	err  error
}

func (f fakeSelects) FetchSelects(_ context.Context, report string) (map[string][]dooglys.SelectOption, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.data[report], nil
}

func liveStore(t *testing.T) *Store {
	t.Helper()
	src := fakeSelects{data: map[string]map[string][]dooglys.SelectOption{
		"payment": {
			"locality_id": {
				{UUID: "loc-vyksa", Name: "Выкса осн"},
				{UUID: "loc-kazan", Name: "Казань"},
			},
			"sale_point_id": {
				{UUID: "sp-vokzal", Name: "Казанский вокзал"},
				{UUID: "sp-dup", Name: "Казанский вокзал"}, // дубль имени — допустим
			},
		},
	}}
	s, err := NewLiveStore(context.Background(), src)
	if err != nil {
		t.Fatalf("NewLiveStore: %v", err)
	}
	return s
}

// Живые uuid из select-формы резолвятся напрямую (locality_id → kind locality).
func TestLiveStoreResolvesLocality(t *testing.T) {
	m, err := liveStore(t).Resolve("locality", "Казань")
	if err != nil {
		t.Fatalf("резолв: %v", err)
	}
	if m.UUID != "loc-kazan" {
		t.Errorf("uuid=%q, want loc-kazan", m.UUID)
	}
}

// param sale_point_id маппится в kind sale_point.
func TestLiveStoreResolvesSalePoint(t *testing.T) {
	m, err := liveStore(t).Resolve("sale_point", "Казанский вокзал")
	if err != nil {
		t.Fatalf("резолв: %v", err)
	}
	if m.UUID != "sp-vokzal" {
		t.Errorf("uuid=%q, want sp-vokzal (первый из дублей)", m.UUID)
	}
}

// Пустой ответ источника → ошибка (вызывающий упадёт на fixture-fallback).
func TestLiveStoreEmpty(t *testing.T) {
	src := fakeSelects{data: map[string]map[string][]dooglys.SelectOption{"payment": {}}}
	if _, err := NewLiveStore(context.Background(), src); err == nil {
		t.Fatal("ожидалась ошибка при пустых справочниках")
	}
}
