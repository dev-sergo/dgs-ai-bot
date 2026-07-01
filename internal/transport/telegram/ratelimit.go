package telegram

import (
	"sync"
	"time"
)

// Дефолты анти-спам лимитера: не более rateLimitRequests запросов за rateLimitWindow
// на один chatID. Порог поверх капа параллелизма (8 одновременных Ask в Run) — тот
// защищает процесс от нагрузки, этот защищает LLM от «тенант лупит от души» (риск,
// который прямо отметил заказчик, docs/12 §4 P0 #7).
const (
	rateLimitRequests = 10
	rateLimitWindow   = time.Minute
)

// rateLimiter — пер-чат ограничитель частоты (скользящее окно). Хранит отметки времени
// недавних запросов на chatID и пропускает, пока их меньше limit за window. Потокобезопасен
// (Run обрабатывает сообщения в горутинах). now инъектируется для детерминированных тестов.
type rateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu   sync.Mutex
	hits map[int64][]time.Time
}

// newRateLimiter создаёт лимитер. limit<=0 → лимитер выключен (allow всегда true).
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		limit:  limit,
		window: window,
		now:    time.Now,
		hits:   make(map[int64][]time.Time),
	}
}

// allow регистрирует попытку chatID и сообщает, пропущена ли она. Прунит отметки
// старше окна; при переполнении возвращает false, НЕ добавляя новую отметку — иначе
// непрерывный спам бесконечно продлевал бы окно и лимит никогда не сбрасывался бы.
// nil-приёмник или limit<=0 → лимитер выключен (пропускает всё): тесты чистых решений
// строят Bot без лимитера.
func (rl *rateLimiter) allow(chatID int64) bool {
	if rl == nil || rl.limit <= 0 {
		return true
	}
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := rl.now()
	cutoff := now.Add(-rl.window)
	// Фильтр «на месте»: пишем только свежие отметки в тот же backing-массив (индекс
	// записи всегда <= индекса чтения, перезапись безопасна).
	recent := rl.hits[chatID][:0]
	for _, t := range rl.hits[chatID] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= rl.limit {
		rl.hits[chatID] = recent
		return false
	}
	rl.hits[chatID] = append(recent, now)
	return true
}
