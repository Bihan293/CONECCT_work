package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"

	tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

var inFlight = struct {
	mu sync.Mutex
	m  map[int64]string
}{m: map[int64]string{}}

// webhook handler factory
func makeWebhookHandler(b *Bot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		var upd tgbot.Update
		if err := json.Unmarshal(body, &upd); err != nil {
			w.WriteHeader(400)
			return
		}
		// quick ack
		go processUpdate(b, &upd)
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

	// check if user is in a flow (executor creating profile or client writing order)
	inFlight.mu.Lock()
	state := inFlight.m[uid]
	inFlight.mu.Unlock()

	if state == "creating_profile" {
		// enforce 150-200 symbols
		txt := strings.TrimSpace(msg.Text)
		if len([]rune(txt)) < 150 || len([]rune(txt)) > 200 {
			sendText(b, chatID, "Описание должно быть от 150 до 200 символов. Попробуйте снова.")
			return
		}
		var photoFileID string
		if msg.Photo != nil && len(msg.Photo) > 0 {
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
		// state: creating_order:<category>
		parts := strings.Split(state, ":")
		cat := parts[1]
		txt := strings.TrimSpace(msg.Text)
		if txt == "" {
			sendText(b, chatID, "Опишите задачу текстом.")
			return
		}
		var photoFileID string
		if msg.Photo != nil && len(msg.Photo) > 0 {
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
		// send to group
		sendOrderToGroup(b, id, ord)
		inFlight.mu.Lock()
		delete(inFlight.m, uid)
		inFlight.mu.Unlock()
		sendText(b, chatID, "Анкета создана и отправлена в группу.")
		return
	}

	// default fallback
	sendText(b, chatID, "Нажмите /start чтобы начать.")
}

func handleCallback(b *Bot, q *tgbot.CallbackQuery) {
	data := q.Data
	uid := q.From.ID
	chatID := q.Message.Chat.ID

	// acknowledge
	b.api.Request(tgbot.NewCallback(q.ID, ""))

	if data == "role:executor" {
		// start profile creation flow
		inFlight.mu.Lock()
		inFlight.m[uid] = "creating_profile"
		inFlight.mu.Unlock()
		sendText(b, int64(uid), "Пришлите описание профиля (150-200 символов). Можно отправить фото вместе с описанием.")
		return
	}
	if data == "role:client" {
		// send categories
		msg := tgbot.NewMessage(chatID, "Выберите нишу:")
		msg.ReplyMarkup = categoriesKeyboard()
		b.Send(msg)
		return
	}
	if strings.HasPrefix(data, "cat:") {
		cat := strings.Split(data, ":")[1]
		// mark state
		inFlight.mu.Lock()
		inFlight.m[uid] = "creating_order:" + cat
		inFlight.mu.Unlock()
		sendText(b, int64(uid), "Опишите задачу и (опционально) прикрепите фото. Пример: Хочу сайт-визитку, бюджет 20000.")
		return
	}
	if strings.HasPrefix(data, "order:connect:") {
		idstr := strings.Split(data, ":")[2]
		id, _ := strconv.ParseInt(idstr, 10, 64)
		handleConnect(b, uid, id)
		return
	}
	if strings.HasPrefix(data, "order:complain:") {
		idstr := strings.Split(data, ":")[2]
		id, _ := strconv.ParseInt(idstr, 10, 64)
		// ask for confirmation
		btn := tgbot.NewInlineKeyboardMarkup(tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("Да, пожаловаться", fmt.Sprintf("complain:confirm:%d", id)),
			tgbot.NewInlineKeyboardButtonData("Отмена", "complain:cancel"),
		))
		msg := tgbot.NewMessage(chatID, "Вы уверены, что хотите отправить жалобу на эту анкету?")
		msg.ReplyMarkup = btn
		b.Send(msg)
		return
	}
	if strings.HasPrefix(data, "complain:confirm:") {
		idstr := strings.Split(data, ":")[2]
		id, _ := strconv.ParseInt(idstr, 10, 64)
		c, err := storage.IncrementComplaint(id, uid)
		if err != nil {
			sendText(b, int64(uid), "Ошибка при отправке жалобы.")
			return
		}
		sendText(b, int64(uid), "Жалоба принята. Количество жалоб: "+strconv.Itoa(c))
		// if >=10 delete order
		if c >= 10 {
			od, _ := storage.GetOrderByID(id)
			if od != nil {
				_ = storage.DeleteOrderByID(id)
				// notify creator
				sendText(b, od.CreatorID, "Ваша анкета была удалена из-за 10 жалоб.")
			}
		} else if c >= 7 {
			// warn author (example threshold)
			od, _ := storage.GetOrderByID(id)
			if od != nil {
				sendText(b, od.CreatorID, fmt.Sprintf("Ваша анкета получила %d жалоб. Если жалоб станет 10 — она будет удалена.", c))
			}
		}
		return
	}
	if data == "complain:cancel" {
		sendText(b, int64(uid), "Жалоба отменена.")
		return
	}
}

func sendText(b *Bot, chatID int64, text string) {
	msg := tgbot.NewMessage(chatID, text)
	b.Send(msg)
}

func sendStart(b *Bot, chatID int64) {
	msg := tgbot.NewMessage(chatID, "Кто вы? Выберите роль:")
	msg.ReplyMarkup = startKeyboard()
	b.Send(msg)
}

func sendProfileToChat(b *Bot, chatID int64, p Profile) {
	txt := fmt.Sprintf("Профиль @%s\n\n%s", p.Username, p.Description)
	msg := tgbot.NewMessage(chatID, txt)
	b.Send(msg)
	if p.PhotoFileID != "" {
		ph := tgbot.NewPhoto(chatID, tgbot.FileID(p.PhotoFileID))
		b.Send(ph)
	}
}

func deleteOrderByCreator(userID int64) error {
	od, err := storage.GetOrderByCreator(userID)
	if err != nil {
		return err
	}
	return storage.DeleteOrderByID(od.ID)
}

func sendOrderToGroup(b *Bot, orderID int64, ord Order) {
	cfg := LoadConfigFromEnv()
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
	// attach buttons
	msg.ReplyMarkup = orderButtonsInline(orderID)
	b.Send(msg)
	// store group message id is optional (not implemented for JSON)
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
	// send notification to creator
	sendText(b, od.CreatorID, fmt.Sprintf("Ваша анкета принята пользователем %d", connectorID))
	// if connector has profile — send it to creator
	if prof, err := storage.GetProfile(connectorID); err == nil && prof != nil {
		sendProfileToChat(b, od.CreatorID, *prof)
	}
	// delete order
	_ = storage.DeleteOrderByID(orderID)
	// confirm connector
	sendText(b, int64(connectorID), "Вы успешно сконнектились с автором анкеты.")
}
