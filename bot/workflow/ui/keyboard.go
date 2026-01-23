package ui

import (
	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
)

// YesNoKeyboard creates an inline keyboard with Yes/No buttons.
func YesNoKeyboard(yesText, noText string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{
				{Text: yesText, CallbackData: "wf:yes"},
				{Text: noText, CallbackData: "wf:no"},
			},
		},
	}
}

// ConfirmCancelKeyboard creates an inline keyboard with Confirm/Cancel buttons.
func ConfirmCancelKeyboard(confirmText, cancelText string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{
				{Text: confirmText, CallbackData: "wf:confirm"},
				{Text: cancelText, CallbackData: "wf:cancel"},
			},
		},
	}
}

// ContactRequestKeyboard creates a reply keyboard with a contact request button.
func ContactRequestKeyboard(buttonText string) tgbotapi.ReplyKeyboardMarkup {
	return tgbotapi.ReplyKeyboardMarkup{
		Keyboard: [][]tgbotapi.KeyboardButton{
			{
				{Text: buttonText, RequestContact: true},
			},
		},
		ResizeKeyboard:  true,
		OneTimeKeyboard: true,
	}
}

// RemoveKeyboard creates a remove keyboard markup to hide custom keyboards.
func RemoveKeyboard() tgbotapi.ReplyKeyboardRemove {
	return tgbotapi.ReplyKeyboardRemove{
		RemoveKeyboard: true,
	}
}

// SingleButtonKeyboard creates an inline keyboard with a single button.
func SingleButtonKeyboard(text, callbackData string) tgbotapi.InlineKeyboardMarkup {
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{
			{
				{Text: text, CallbackData: callbackData},
			},
		},
	}
}

// ButtonRow creates a row of inline buttons from a map of text->callbackData.
func ButtonRow(buttons map[string]string) []tgbotapi.InlineKeyboardButton {
	row := make([]tgbotapi.InlineKeyboardButton, 0, len(buttons))
	for text, data := range buttons {
		row = append(row, tgbotapi.InlineKeyboardButton{
			Text:         text,
			CallbackData: data,
		})
	}
	return row
}

// SelectableItem represents an item that can be selected from a list.
type SelectableItem struct {
	ID   string
	Text string
}

// SelectionKeyboard creates an inline keyboard for selecting items.
// Each item gets its own row with callback data in format "wf:select:ID".
func SelectionKeyboard(items []SelectableItem) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, len(items))
	for i, item := range items {
		rows[i] = []tgbotapi.InlineKeyboardButton{
			{Text: item.Text, CallbackData: "wf:select:" + item.ID},
		}
	}
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

// MainMenuKeyboard creates a keyboard for the main menu.
func MainMenuKeyboard(buttons [][]SelectableItem) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, len(buttons))
	for i, row := range buttons {
		rowButtons := make([]tgbotapi.InlineKeyboardButton, len(row))
		for j, item := range row {
			rowButtons[j] = tgbotapi.InlineKeyboardButton{
				Text:         item.Text,
				CallbackData: "wf:menu:" + item.ID,
			}
		}
		rows[i] = rowButtons
	}
	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}
