package telegram

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

const msgStart = `👋 <b>Привет! Я AI-аналитик вашего заведения.</b>

<b>Примеры запросов:</b>
• Как вчера отработали?
• Покажи выручку за прошлую неделю
• На чём я теряю деньги?
• Топ-5 товаров за май
• Выгрузи таблицу за май

<b>Команды:</b> /help, /today, /tt КОД_ТТ`

const msgHelp = `<b>Команды</b>
/start — начало
/help  — справка
/today — дайджест за вчера
/tt КОД — фокус на торговой точке (напр. /tt IV-001)

Пишите текстом — подтяну данные и отвечу с цифрами.`

// handleCommand маршрутизирует команды бота.
func (b *Bot) handleCommand(ctx context.Context, msg *tgbotapi.Message) {
	switch msg.Command() {
	case "start":
		b.sendHTML(msg.Chat.ID, msgStart)
	case "help":
		b.sendHTML(msg.Chat.ID, msgHelp)
	case "today":
		b.ask(ctx, msg.Chat.ID, "как вчера отработали?")
	case "tt":
		b.handleTT(ctx, msg)
	default:
		b.send(msg.Chat.ID, "Неизвестная команда. /help — список команд.")
	}
}

// handleTT обрабатывает /tt КОД: сохраняет фильтр точки в сессии через Ask.
// Код передаётся как уточняющий контекст в следующем вопросе к Ask, а не как
// отдельный механизм: бот спрашивает «выручка за сегодня по точке КОД?» и
// сессия подхватывает фильтр sale_point через обычный NLU-путь.
func (b *Bot) handleTT(ctx context.Context, msg *tgbotapi.Message) {
	code := strings.TrimSpace(msg.CommandArguments())
	if code == "" {
		sessionID := fmt.Sprintf("tg:%d", msg.Chat.ID)
		_ = sessionID
		b.send(msg.Chat.ID, "Укажите код точки: /tt IV-001")
		return
	}
	b.send(msg.Chat.ID, fmt.Sprintf("📍 Фокус на точке %s. Следующие вопросы буду отвечать по ней.", code))
	// Инициируем запрос с явным кодом точки — сессия запомнит фильтр через NLU.
	b.ask(ctx, msg.Chat.ID, fmt.Sprintf("выручка за сегодня по точке %s", code))
}
