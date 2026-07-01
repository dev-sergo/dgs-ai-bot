package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"dgsbot/internal/app"
	"dgsbot/internal/config"
)

// Bot — Telegram-транспорт поверх app.Ask, привязанный к ОДНОМУ тенанту (3-бот режим).
// Polling-loop, allowlist-guard (свой на бот/тенант), рендер Answer.
//
// Шов к «1 бот на все тенанты»: топология чат→тенант живёт в resolveTenant. Сейчас это
// константа (бот жёстко на своём тенанте); в будущем — замена на chatID→tenant-резолвер,
// без изменения app/клиента (движок уже тенант-агностичен, tenantID приходит в Ask).
type Bot struct {
	api       *tgbotapi.BotAPI
	app       *app.App
	tenantID  string
	allowlist map[int64]struct{}
	limiter   *rateLimiter
}

// New создаёт Bot, привязанный к тенанту tenantID, со своим токеном и whitelist'ом.
// Возвращает ошибку если токен невалиден.
func New(token, tenantID string, allowlist []int64, a *app.App) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}

	al := make(map[int64]struct{}, len(allowlist))
	for _, id := range allowlist {
		al[id] = struct{}{}
	}

	slog.Info("telegram bot started", "username", api.Self.UserName,
		"tenant", tenantID, "allowlist", len(allowlist))
	return &Bot{
		api:       api,
		app:       a,
		tenantID:  tenantID,
		allowlist: al,
		limiter:   newRateLimiter(rateLimitRequests, rateLimitWindow),
	}, nil
}

// NewFromTenant — удобный конструктор из config.TenantConfig (bootstrap N ботов).
func NewFromTenant(tc config.TenantConfig, a *app.App) (*Bot, error) {
	return New(tc.BotToken, tc.ID, tc.Allowlist, a)
}

// resolveTenant — шов резолва чата в тенант. В 3-бот режиме бот жёстко на своём
// тенанте, поэтому chatID игнорируется. Для «1 бот на все тенанты» здесь встанет
// маппинг chatID→tenant (единственная точка изменения топологии).
func (b *Bot) resolveTenant(_ int64) string { return b.tenantID }

// allowed сообщает, пропущен ли chatID whitelist'ом бота. Пустой список → открыт всем
// (как пустой AUTH_TOKEN выключает HTTP-гейт). Чужой chat_id → false.
func (b *Bot) allowed(chatID int64) bool {
	if len(b.allowlist) == 0 {
		return true
	}
	_, ok := b.allowlist[chatID]
	return ok
}

// Run запускает polling-loop; блокирует до отмены ctx.
func (b *Bot) Run(ctx context.Context) {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 30
	updates := b.api.GetUpdatesChan(u)

	// Ограничиваем параллелизм: не более 8 одновременных вызовов Ask.
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			b.api.StopReceivingUpdates()
			wg.Wait()
			return
		case upd, ok := <-updates:
			if !ok {
				return
			}
			switch {
			case upd.Message != nil:
				sem <- struct{}{}
				wg.Add(1)
				go func(msg *tgbotapi.Message) {
					defer wg.Done()
					defer func() { <-sem }()
					b.handle(ctx, msg)
				}(upd.Message)
			case upd.CallbackQuery != nil:
				go b.handleCallback(upd.CallbackQuery)
			}
		}
	}
}

// handle обрабатывает одно входящее сообщение.
func (b *Bot) handle(ctx context.Context, msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	// Allowlist-guard: свой whitelist на бот/тенант. Чужой chat_id отбит.
	if !b.allowed(chatID) {
		b.send(chatID, "Доступ закрыт.")
		return
	}

	// Анти-спам: пер-чат частотный лимит поверх капа параллелизма. Превышение —
	// мягкий ответ (не тихий дроп), чтобы пользователь понял, что надо подождать.
	if !b.limiter.allow(chatID) {
		b.send(chatID, "Слишком много запросов. Подождите немного и повторите.")
		return
	}

	text := msg.Text
	if text == "" {
		return
	}

	// Команды обрабатываются отдельно.
	if msg.IsCommand() {
		b.handleCommand(ctx, msg)
		return
	}

	b.ask(ctx, chatID, text)
}

// ask вызывает app.Ask и отправляет результат в чат с кнопками 👍/👎.
func (b *Bot) ask(ctx context.Context, chatID int64, text string) {
	typing := tgbotapi.NewChatAction(chatID, tgbotapi.ChatTyping)
	_, _ = b.api.Send(typing)

	tenantID := b.resolveTenant(chatID)
	// Сессия скоупится тенантом: N ботов в одном процессе делят один session.Store,
	// а один и тот же chatID может быть в whitelist нескольких ботов — без префикса
	// история/last-plan тенантов слились бы (кросс-утечка контекста).
	sessionID := fmt.Sprintf("tg:%s:%d", tenantID, chatID)

	ans, err := b.app.Ask(ctx, tenantID, sessionID, text)
	if err != nil {
		slog.Error("telegram ask error", "chat_id", chatID, "err", err)
		b.send(chatID, "Произошла ошибка при обращении к данным. Попробуйте позже.")
		return
	}

	replyText, doc := Render(ans)
	b.sendWithFeedback(chatID, replyText, ans.ID)

	if doc != nil {
		file := tgbotapi.FileBytes{Name: doc.Name, Bytes: doc.Data}
		docMsg := tgbotapi.NewDocument(chatID, file)
		if _, err := b.api.Send(docMsg); err != nil {
			slog.Error("telegram send document error", "chat_id", chatID, "err", err)
		}
	}
}

// sendWithFeedback отправляет текст с инлайн-кнопками 👍/👎 (если id непустой).
func (b *Bot) sendWithFeedback(chatID int64, text, answerID string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if answerID != "" {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("👍", "fb:up:"+answerID),
				tgbotapi.NewInlineKeyboardButtonData("👎", "fb:down:"+answerID),
			),
		)
	}
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send error", "chat_id", chatID, "err", err)
	}
}

// handleCallback обрабатывает тап по инлайн-кнопке 👍/👎.
// Формат callback data: "fb:<rating>:<answerID>".
func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	parts := strings.SplitN(cb.Data, ":", 3)
	if len(parts) != 3 || parts[0] != "fb" {
		return
	}
	rating, answerID := parts[1], parts[2]
	if rating != "up" && rating != "down" {
		return
	}
	ts := time.Now().UTC().Format(time.RFC3339)
	b.app.RecordFeedback(ts, answerID, rating, "telegram")

	// Подтверждение тапа — всплывашка над клавиатурой.
	ack := tgbotapi.NewCallback(cb.ID, "Спасибо!")
	_, _ = b.api.Request(ack)
}

// send отправляет текстовое сообщение. Ошибку логирует, не паникует.
func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send error", "chat_id", chatID, "err", err)
	}
}
