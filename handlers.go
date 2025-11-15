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
var updatesChan = make(chan *tgbot.Update, 100) // буфер для апдейтов
var messagesChan = make(chan tgbot.Chattable, 100) // канал для сообщений

func startWorkers(b *Bot, updateWorkers int, msgWorkers int) {
	// воркеры для апдейтов
	for i := 0; i < updateWorkers; i++ {
		go func() {
			for upd := range updatesChan {
				processUpdate(b, upd)
			}
		}()
	}

	// воркеры для отправки сообщений
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

// периодическая чистка старых состояний
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

func sendStart(b *Bot, chatID int64) {
	msg := tgbot.NewMessage(chatID, "Кто вы? Выберите роль:")
	msg.ReplyMarkup = startKeyboard()
	sendMessage(msg)
}

func sendProfileToChat(b *Bot, chatID int64, p Profile) {
	txt := fmt.Sprintf("Профиль @%s\n\n%s", p.Username, p.Description)
	sendText(b, chatID, txt)
	if p.PhotoFileID != "" {
		sendMessage(tgbot.NewPhoto(chatID, tgbot.FileID(p.PhotoFileID)))
	}
}

// ------------------------ Message handlers ------------------------
func handleMessage(b *Bot, msg *tgbot.Message) {
	chatID := msg.Chat.ID
	uid := msg.From.ID

	// handle commands
	if msg.IsCommand() {
		switch msg.Command() {
		case "start":
			sendStart(b, chatID)
		case "delete_order":
			if err := deleteOrderByCreator(uid); err != nil {
				sendText(b, chatID, "У вас нет активной анкеты.")
			} else {
				sendText(b, chatID, "Ваша анкета удалена.")
			}
		case "my_profile":
			p, err := storage.GetProfile(uid)
			if err != nil {
				sendText(b, chatID, "Профиль не найден.")
				return
			}
			sendProfileToChat(b, chatID, *p)
		default:
			sendText(b, chatID, "Неизвестная команда.")
		}
		return
	}

	// check inFlight
	inFlight.mu.Lock()
	stateObj, ok := inFlight.m[uid]
	inFlight.mu.Unlock()

	state := ""
	if ok {
		state = stateObj.state
	}

	if state == "creating_profile" {
		txt := strings.TrimSpace(msg.Text)
		if len([]rune(txt)) < 150 || len([]rune(txt)) > 200 {
			sendText(b, chatID, "Описание должно быть от 150 до 200 символов. Попробуйте снова.")
			return
		}
		var photoFileID string
		if len(msg.Photo) > 0 {
			photoFileID = msg.Photo[len(msg.Photo)-1].FileID
		}
		prof := Profile{
			UserID:      uid,
			Username:    msg.From.UserName,
			Description: txt,
			PhotoFileID: photoFileID,
		}
		if err := storage.CreateOrUpdateProfile(prof); err != nil {
			sendText(b, chatID, "Ошибка при сохранении профиля.")
			return
		}
		inFlight.mu.Lock()
		delete(inFlight.m, uid)
		inFlight.mu.Unlock()
		sendText(b, chatID, "Профиль сохранен. Вы можете отредактировать его командой /my_profile")
		return
	}

	if strings.HasPrefix(state, "creating_order:") {
		parts := strings.Split(state, ":")
		cat := parts[1]
		txt := strings.TrimSpace(msg.Text)
		if txt == "" {
			sendText(b, chatID, "Опишите задачу текстом.")
			return
		}
		var photoFileID string
		if len(msg.Photo) > 0 {
			photoFileID = msg.Photo[len(msg.Photo)-1].FileID
		}
		ord := Order{
			CreatorID:   uid,
			Category:    cat,
			Text:        txt,
			PhotoFileID: photoFileID,
		}
		id, err := storage.CreateOrder(ord)
		if err != nil {
			sendText(b, chatID, "У вас уже есть активная анкета. Удалите её перед созданием новой.")
			inFlight.mu.Lock()
			delete(inFlight.m, uid)
			inFlight.mu.Unlock()
			return
		}
		sendOrderToGroup(b, id, ord)
		inFlight.mu.Lock()
		delete(inFlight.m, uid)
		inFlight.mu.Unlock()
		sendText(b, chatID, "Анкета создана и отправлена в группу.")
		return
	}

	sendText(b, chatID, "Нажмите /start чтобы начать.")
}

// ------------------------ Callbacks ------------------------
func handleCallback(b *Bot, q *tgbot.CallbackQuery) {
	data := q.Data
	uid := q.From.ID
	chatID := q.Message.Chat.ID

	b.api.Request(tgbot.NewCallback(q.ID, "")) // acknowledge

	switch {
	case data == "role:executor":
		inFlight.mu.Lock()
		inFlight.m[uid] = userState{"creating_profile", time.Now()}
		inFlight.mu.Unlock()
		sendText(b, int64(uid), "Пришлите описание профиля (150-200 символов). Можно отправить фото вместе с описанием.")
	case data == "role:client":
		msg := tgbot.NewMessage(chatID, "Выберите нишу:")
		msg.ReplyMarkup = categoriesKeyboard()
		sendMessage(msg)
	case strings.HasPrefix(data, "cat:"):
		cat := strings.Split(data, ":")[1]
		inFlight.mu.Lock()
		inFlight.m[uid] = userState{"creating_order:" + cat, time.Now()}
		inFlight.mu.Unlock()
		sendText(b, int64(uid), "Опишите задачу и (опционально) прикрепите фото. Пример: Хочу сайт-визитку, бюджет 20000.")
	case strings.HasPrefix(data, "order:connect:"):
		idstr := strings.Split(data, ":")[2]
		id, _ := strconv.ParseInt(idstr, 10, 64)
		handleConnect(b, uid, id)
	case strings.HasPrefix(data, "order:complain:"):
		idstr := strings.Split(data, ":")[2]
		id, _ := strconv.ParseInt(idstr, 10, 64)
		btn := tgbot.NewInlineKeyboardMarkup(tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("Да, пожаловаться", fmt.Sprintf("complain:confirm:%d", id)),
			tgbot.NewInlineKeyboardButtonData("Отмена", "complain:cancel"),
		))
		msg := tgbot.NewMessage(chatID, "Вы уверены, что хотите отправить жалобу на эту анкету?")
		msg.ReplyMarkup = btn
		sendMessage(msg)
	case strings.HasPrefix(data, "complain:confirm:"):
		idstr := strings.Split(data, ":")[2]
		id, _ := strconv.ParseInt(idstr, 10, 64)
		c, err := storage.IncrementComplaint(id, uid)
		if err != nil {
			sendText(b, int64(uid), "Ошибка при отправке жалобы.")
			return
		}
		sendText(b, int64(uid), "Жалоба принята. Количество жалоб: "+strconv.Itoa(c))
		if c >= 10 {
			if od, _ := storage.GetOrderByID(id); od != nil {
				_ = storage.DeleteOrderByID(id)
				sendText(b, od.CreatorID, "Ваша анкета была удалена из-за 10 жалоб.")
			}
		} else if c >= 7 {
			if od, _ := storage.GetOrderByID(id); od != nil {
				sendText(b, od.CreatorID, fmt.Sprintf("Ваша анкета получила %d жалоб. Если жалоб станет 10 — она будет удалена.", c))
			}
		}
	case data == "complain:cancel":
		sendText(b, int64(uid), "Жалоба отменена.")
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

func sendOrderToGroup(b *Bot, orderID int64, ord Order) {
	cfg := LoadConfigFromEnv() // лучше один раз грузить в main
	var gid int64
	switch ord.Category {
	case "design":
		gid = cfg.DesignGroupID
	case "programming":
		gid = cfg.ProgrammingGroupID
	default:
		gid = cfg.ContentGroupID
	}
	txt := fmt.Sprintf("Новая анкета (id %d)\nКатегория: %s\nТекст: %s\nОт: %d", orderID, ord.Category, ord.Text, ord.CreatorID)
	msg := tgbot.NewMessage(gid, txt)
	msg.ReplyMarkup = orderButtonsInline(orderID)
	sendMessage(msg)
}

func orderButtonsInline(id int64) tgbot.InlineKeyboardMarkup {
	connect := tgbot.NewInlineKeyboardButtonData("🔗 Коннект", "order:connect:"+strconv.FormatInt(id, 10))
	complain := tgbot.NewInlineKeyboardButtonData("🚫 Жалоба", "order:complain:"+strconv.FormatInt(id, 10))
	return tgbot.NewInlineKeyboardMarkup(tgbot.NewInlineKeyboardRow(connect, complain))
}

func handleConnect(b *Bot, connectorID int64, orderID int64) {
	od, err := storage.GetOrderByID(orderID)
	if err != nil {
		sendText(b, int64(connectorID), "Анкета не найдена.")
		return
	}
	sendText(b, od.CreatorID, fmt.Sprintf("Ваша анкета принята пользователем %d", connectorID))
	if prof, err := storage.GetProfile(connectorID); err == nil && prof != nil {
		sendProfileToChat(b, od.CreatorID, *prof)
	}
	_ = storage.DeleteOrderByID(orderID)
	sendText(b, int64(connectorID), "Вы успешно сконнектились с автором анкеты.")
}
