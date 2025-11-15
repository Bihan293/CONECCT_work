package main

import tgbot "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// Стартовые кнопки: Исполнитель и Клиент (рядом)
func startKeyboard() tgbot.InlineKeyboardMarkup {
	return tgbot.NewInlineKeyboardMarkup(
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("👷 Исполнитель", "role:executor"),
			tgbot.NewInlineKeyboardButtonData("🧑‍💼 Клиент", "role:client"),
		),
	)
}

// Кнопки для категорий при выборе клиента (каждая на отдельном ряду)
func categoriesKeyboard() tgbot.InlineKeyboardMarkup {
	return tgbot.NewInlineKeyboardMarkup(
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("🎨 Дизайн", "cat:design"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("💻 Программирование", "cat:programming"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("📸 Контент-мейкинг", "cat:content"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("🔙 Назад", "back:to_start"),
		),
	)
}

// Кнопки для профиля (редактировать/удалить)
func profileKeyboard() tgbot.InlineKeyboardMarkup {
	return tgbot.NewInlineKeyboardMarkup(
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("✏️ Редактировать профиль", "profile:edit"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("🗑️ Удалить профиль", "profile:delete"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("🔙 Назад", "back:to_start"),
		),
	)
}

// Кнопки для групп работы после создания профиля (для исполнителя)
func groupsKeyboard() tgbot.InlineKeyboardMarkup {
	return tgbot.NewInlineKeyboardMarkup(
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("🎨 Дизайн", "group:design"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("💻 Программирование", "group:programming"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("📸 Контент-мейкинг", "group:content"),
		),
		tgbot.NewInlineKeyboardRow(
			tgbot.NewInlineKeyboardButtonData("🔙 Назад", "back:to_profile"),
		),
	)
}
