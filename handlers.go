package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// ------------------------ Worker pool ------------------------
var updatesChan = make(chan *tgbot.Update, 100)
var messagesChan = make(chan tgbot.Chattable, 100)

func startWorkers(b *Bot, updateWorkers int, msgWorkers int) {
	for i := 0; i < updateWorkers; i++ {
		go func() {
			for upd := range updatesChan {
				processUpdate(b, upd)
			}
		}()
	}
	for i := 0; i < msgWorkers; i++ {
		go func() {
			for msg := range messagesChan {
				b.Send(msg)
			}
		}()
	}
}

// ------------------------ InFlight ------------------------
type userState struct {
	state string
	ts    time.Time
}

var inFlight = struct {
	mu sync.Mutex
	m  map[int64]userState
}{m: map[int64]userState{}}

func startInFlightCleaner() {
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			inFlight.mu.Lock()
			now := time.Now()
			for uid, s := range inFlight.m {
				if now.Sub(s.ts) > 15*time.Minute {
					delete(inFlight.m, uid)
				}
			}
			inFlight.mu.Unlock()
		}
	}()
}

// ------------------------ Webhook ------------------------
func makeWebhookHandler(b *Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var upd tgbot.Update
		if err := json.Unmarshal(body, &upd); err != nil {
			w.WriteHeader(400)
			return
		}
		updatesChan <- &upd
		w.WriteHeader(200)
	}
}

func processUpdate(b *Bot, upd *tgbot.Update) {
	if upd.Message != nil {
		handleMessage(b, upd.Message)
	} else if upd.CallbackQuery != nil {
		handleCallback(b, upd.CallbackQuery)
	}
}

// ------------------------ Message sending ------------------------
func sendMessage(msg tgbot.Chattable) {
	messagesChan <- msg
}

func sendText(b *Bot, chatID int64, text string) {
	sendMessage(tgbot.NewMessage(chatID, text))
}

// ------------------------ Keyboards ------------------------
func startKeyboard() tgbot.ReplyKeyboardMarkup {
	return tgbot.NewReplyKeyboard(
		tgbot.NewKeyboardButtonRow(
			tgbot.NewKeyboardButton("👷 Исполнитель"),
			tgbot.NewKeyboardButton("🧑 Клиент"),
		),
	)
}

func profileOptionsKeyboard() tgbot.ReplyKeyboardMarkup {
	return tgbot.NewReplyKeyboard(
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("🔄 Редактировать профиль")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("🗑 Удалить профиль")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("🎨 Дизайн")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("💻 Программирование")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("✍️ Контент")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("↩️ Назад")),
	)
}

func orderOptionsKeyboard(category string) tgbot.ReplyKeyboardMarkup {
	return tgbot.NewReplyKeyboard(
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("🔄 Редактировать анкету")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("🗑 Удалить анкету")),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton(categoryEmoji(category) + " " + category)),
		tgbot.NewKeyboardButtonRow(tgbot.NewKeyboardButton("↩️ Назад")),
	)
}

func categoryEmoji(cat string) string {
	switch cat {
	case "design":
		return "🎨"
	case "programming":
		return "💻"
	default:
		return "✍️"
	}
}

// ------------------------ Message handlers ------------------------
func handleMessage(b *Bot, msg *tgbot.Message) {
	chatID := msg.Chat.ID
	uid := msg.From.ID

	text := strings.TrimSpace(msg.Text)

	// Команды
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			sendText(b, chatID, "Выберите роль:")
			sendMessage(tgbot.NewMessage(chatID, "Выберите роль:"))
			return
		case "my_profile":
			p, err := storage.GetProfile(uid)
			if err != nil || p == nil {
				sendText(b, chatID, "Профиль не найден.")
				return
			}
			sendProfileToChat(b, chatID, *p)
			sendMessage(tgbot.NewMessage(chatID, "Выберите опцию:", profileOptionsKeyboard()))
			return
		case "delete_order":
			if err := deleteOrderByCreator(uid); err != nil {
				sendText(b, chatID, "У вас нет активной анкеты.")
			} else {
				sendText(b, chatID, "Ваша анкета удалена.")
			}
			return
		}
	}

	inFlight.mu.Lock()
	stateObj, ok := inFlight.m[uid]
	inFlight.mu.Unlock()
	state := ""
	if ok {
		state = stateObj.state
	}

	switch {
	case state == "creating_profile":
		var photo string
		if len(msg.Photo) > 0 {
			photo = msg.Photo[len(msg.Photo)-1].FileID
		}
		if len(text) > 100 {
			sendText(b, chatID, "Описание не должно быть длиннее 100 символов.")
			return
		}
		prof := Profile{
			UserID:      uid,
			Username:    msg.From.UserName,
			Description: text,
			PhotoFileID: photo,
		}
		storage.CreateOrUpdateProfile(prof)
		inFlight.mu.Lock()
		delete(inFlight.m, uid)
		inFlight.mu.Unlock()
		sendText(b, chatID, "Профиль сохранен!")
		sendMessage(tgbot.NewMessage(chatID, "Выберите опцию:", profileOptionsKeyboard()))
	case strings.HasPrefix(state, "creating_order:"):
		parts := strings.Split(state, ":")
		category := parts[1]
		if len(text) > 100 && len(msg.Photo) == 0 {
			sendText(b, chatID, "Текст анкеты не должен превышать 100 символов.")
			return
		}
		ord := Order{
			CreatorID:   uid,
			Category:    category,
			Text:        text,
		}
		if len(msg.Photo) > 0 {
			ord.PhotoFileID = msg.Photo[len(msg.Photo)-1].FileID
		}
		if _, err := storage.CreateOrder(ord); err != nil {
			sendText(b, chatID, "У вас уже есть активная анкета. Удалите её перед созданием новой.")
			return
		}
		inFlight.mu.Lock()
		delete(inFlight.m, uid)
		inFlight.mu.Unlock()
		sendText(b, chatID, "Анкета создана!")
		sendMessage(tgbot.NewMessage(chatID, "Ваша анкета:", orderOptionsKeyboard(category)))
	default:
		if text == "↩️ Назад" {
			sendText(b, chatID, "Выберите роль:")
			sendMessage(tgbot.NewMessage(chatID, "Выберите роль:", startKeyboard()))
			return
		}
		sendText(b, chatID, "Нажмите /start, чтобы начать.")
	}
}

// ------------------------ Callbacks ------------------------
func handleCallback(b *Bot, q *tgbot.CallbackQuery) {
	data := q.Data
	uid := q.From.ID
	chatID := q.Message.Chat.ID

	b.api.Request(tgbot.NewCallback(q.ID, ""))

	switch {
	case data == "role:executor":
		inFlight.mu.Lock()
		inFlight.m[uid] = userState{"creating_profile", time.Now()}
		inFlight.mu.Unlock()
		sendText(b, int64(uid), "Отправьте текст (0-100 символов) и/или фото для профиля.")
	case data == "role:client":
		sendText(b, chatID, "Выберите категорию для анкеты:")
		msg := tgbot.NewMessage(chatID, "Выберите категорию:")
		msg.ReplyMarkup = categoriesKeyboard()
		sendMessage(msg)
	case strings.HasPrefix(data, "cat:"):
		category := strings.Split(data, ":")[1]
		inFlight.mu.Lock()
		inFlight.m[uid] = userState{"creating_order:" + category, time.Now()}
		inFlight.mu.Unlock()
		sendText(b, chatID, "Отправьте текст (0-100 символов) и/или фото для анкеты.")
	case strings.HasPrefix(data, "order:connect:"):
		id, _ := strconv.ParseInt(strings.Split(data, ":")[2], 10, 64)
		handleConnect(b, uid, id)
	case strings.HasPrefix(data, "order:complain:"):
		id, _ := strconv.ParseInt(strings.Split(data, ":")[2], 10, 64)
		btn := tgbot.NewInlineKeyboardMarkup(
			tgbot.NewInlineKeyboardRow(
				tgbot.NewInlineKeyboardButtonData("Да, пожаловаться", fmt.Sprintf("complain:confirm:%d", id)),
				tgbot.NewInlineKeyboardButtonData("Отмена", "complain:cancel"),
			),
		)
		msg := tgbot.NewMessage(chatID, "Вы уверены, что хотите отправить жалобу?")
		msg.ReplyMarkup = btn
		sendMessage(msg)
	case strings.HasPrefix(data, "complain:confirm:"):
		id, _ := strconv.ParseInt(strings.Split(data, ":")[2], 10, 64)
		count, err := storage.IncrementComplaint(id, uid)
		if err != nil {
			sendText(b, uid, "Ошибка.")
			return
		}
		sendText(b, uid, fmt.Sprintf("Жалоба принята. Всего: %d", count))
		if count >= 10 {
			if od, _ := storage.GetOrderByID(id); od != nil {
				_ = storage.DeleteOrderByID(id)
				sendText(b, od.CreatorID, "Ваша анкета удалена из-за 10 жалоб.")
			}
		}
	case data == "complain:cancel":
		sendText(b, uid, "Жалоба отменена.")
	}
}

// ------------------------ Orders ------------------------
func deleteOrderByCreator(userID int64) error {
	od, err := storage.GetOrderByCreator(userID)
	if err != nil {
		return err
	}
	return storage.DeleteOrderByID(od.ID)
}

func handleConnect(b *Bot, connectorID int64, orderID int64) {
	od, err := storage.GetOrderByID(orderID)
	if err != nil {
		sendText(b, connectorID, "Анкета не найдена.")
		return
	}
	sendText(b, od.CreatorID, fmt.Sprintf("Ваша анкета принята пользователем %d", connectorID))
	if prof, err := storage.GetProfile(connectorID); err == nil && prof != nil {
		sendProfileToChat(b, od.CreatorID, *prof)
	}
	_ = storage.DeleteOrderByID(orderID)
	sendText(b, connectorID, "Вы успешно сконнектились.")
}
