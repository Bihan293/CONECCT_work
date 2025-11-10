package main

import tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"

func startKeyboard() tgbot.InlineKeyboardMarkup {
	return tgbot.NewInlineKeyboardMarkup(
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("Исполнитель", "role:executor"),
			tgbot.NewInlineKeyboardButtonData("Клиент", "role:client"),
		),
	)
}

func categoriesKeyboard() tgbot.InlineKeyboardMarkup {
	return tgbot.NewInlineKeyboardMarkup(
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("Дизайн", "cat:design"),
			tgbot.NewInlineKeyboardButtonData("Программирование", "cat:programming"),
			tgbot.NewInlineKeyboardButtonData("Контент-мейкинг", "cat:content"),
		),
	)
}
