package telegram

import (
	"context"
	"fmt"
	"html"
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
	api        *tgbotapi.BotAPI
	app        *app.App
	tenantID   string
	allowlist  map[int64]struct{}  // whitelist по числовому chat_id
	allowUsers map[string]struct{} // whitelist по @username (нормализован: без '@', lower)
	limiter    *rateLimiter
}

// New создаёт Bot, привязанный к тенанту tenantID, со своим токеном и whitelist'ом.
// Whitelist смешанный: chat_id (allowlist) и/или @username (allowUsers, уже нормализованы).
// Возвращает ошибку если токен невалиден.
func New(token, tenantID string, allowlist []int64, allowUsers []string, a *app.App) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("telegram: %w", err)
	}

	al := make(map[int64]struct{}, len(allowlist))
	for _, id := range allowlist {
		al[id] = struct{}{}
	}
	au := make(map[string]struct{}, len(allowUsers))
	for _, u := range allowUsers {
		au[u] = struct{}{}
	}

	slog.Info("telegram bot started", "username", api.Self.UserName,
		"tenant", tenantID, "allowlist", len(allowlist)+len(allowUsers))
	return &Bot{
		api:        api,
		app:        a,
		tenantID:   tenantID,
		allowlist:  al,
		allowUsers: au,
		limiter:    newRateLimiter(rateLimitRequests, rateLimitWindow),
	}, nil
}

// NewFromTenant — удобный конструктор из config.TenantConfig (bootstrap N ботов).
func NewFromTenant(tc config.TenantConfig, a *app.App) (*Bot, error) {
	return New(tc.BotToken, tc.ID, tc.Allowlist, tc.AllowUsers, a)
}

// resolveTenant — шов резолва чата в тенант. В 3-бот режиме бот жёстко на своём
// тенанте, поэтому chatID игнорируется. Для «1 бот на все тенанты» здесь встанет
// маппинг chatID→tenant (единственная точка изменения топологии).
func (b *Bot) resolveTenant(_ int64) string { return b.tenantID }

// allowed сообщает, пропущен ли пользователь whitelist'ом бота — по chat_id ИЛИ по
// @username. Оба списка пусты → открыт всем (как пустой AUTH_TOKEN выключает HTTP-гейт).
// Чужой chat_id и username вне списка → false. username сравнивается регистронезависимо.
func (b *Bot) allowed(chatID int64, username string) bool {
	if len(b.allowlist) == 0 && len(b.allowUsers) == 0 {
		return true
	}
	if _, ok := b.allowlist[chatID]; ok {
		return true
	}
	if username != "" {
		if _, ok := b.allowUsers[strings.ToLower(username)]; ok {
			return true
		}
	}
	return false
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

	// Allowlist-guard: свой whitelist на бот/тенант — по chat_id или @username.
	// Чужой отбит. username берём из msg.From (в личке From.ID == Chat.ID).
	var username string
	if msg.From != nil {
		username = msg.From.UserName
	}
	if !b.allowed(chatID, username) {
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

// sendWithFeedback отправляет HTML-текст ответа с инлайн-кнопками 👍/👎 (если id непустой).
// Текст приходит из Render — уже экранированный, с разметкой <b>. Если Telegram
// отверг HTML (битая разметка), повторяем отправку обычным текстом без тегов —
// пользователь получит ответ в любом случае.
func (b *Bot) sendWithFeedback(chatID int64, text, answerID string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = tgbotapi.ModeHTML
	if answerID != "" {
		msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("👍", "fb:up:"+answerID),
				tgbotapi.NewInlineKeyboardButtonData("👎", "fb:down:"+answerID),
			),
		)
	}
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send error (html), retrying plain", "chat_id", chatID, "err", err)
		plain := tgbotapi.NewMessage(chatID, stripHTML(text))
		plain.ReplyMarkup = msg.ReplyMarkup
		if _, err := b.api.Send(plain); err != nil {
			slog.Error("telegram send error", "chat_id", chatID, "err", err)
		}
	}
}

// stripHTML снимает нашу минимальную разметку (<b>) и HTML-экранирование
// для plain-фолбэка.
func stripHTML(s string) string {
	s = strings.NewReplacer("<b>", "", "</b>", "").Replace(s)
	return html.UnescapeString(s)
}

// handleCallback handles a tap on the 👍/👎 inline button.
// Callback data: "fb:<rating>:<answerID>"; "fbnoop" = the locked confirmation button.
func (b *Bot) handleCallback(cb *tgbotapi.CallbackQuery) {
	// Tap on the already-locked confirmation button — no-op, just ack to clear the spinner.
	if cb.Data == "fbnoop" {
		_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, ""))
		return
	}
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

	// Lock the choice: replace the buttons with a static confirmation so the user can't
	// re-vote (otherwise contradictory up/down land in the dataset for one answer id).
	if cb.Message != nil {
		emoji := "👍"
		if rating == "down" {
			emoji = "👎"
		}
		locked := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData("✅ оценка учтена: "+emoji, "fbnoop"),
			),
		)
		_, _ = b.api.Request(tgbotapi.NewEditMessageReplyMarkup(cb.Message.Chat.ID, cb.Message.MessageID, locked))
	}

	// Popup acknowledgement above the keyboard.
	_, _ = b.api.Request(tgbotapi.NewCallback(cb.ID, "Спасибо!"))
}

// send отправляет текстовое сообщение. Ошибку логирует, не паникует.
func (b *Bot) send(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	if _, err := b.api.Send(msg); err != nil {
		slog.Error("telegram send error", "chat_id", chatID, "err", err)
	}
}
