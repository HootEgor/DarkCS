package ui

import (
	"fmt"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
)

const (
	DefaultItemsPerPage = 5
)

// PaginatedList creates an inline keyboard with paginated items.
// Items are displayed 5 per page with navigation buttons.
// Format:
//
//	[Item 1]
//	[Item 2]
//	[Item 3]
//	[Item 4]
//	[Item 5]
//	[◀️ Back] [Page 1/3] [Next ▶️]
func PaginatedList(items []SelectableItem, currentPage, totalPages int) tgbotapi.InlineKeyboardMarkup {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0, len(items)+1)

	// Add item buttons
	for _, item := range items {
		rows = append(rows, []tgbotapi.InlineKeyboardButton{
			{Text: item.Text, CallbackData: "wf:select:" + item.ID},
		})
	}

	// Add navigation row
	navRow := buildNavRow(currentPage, totalPages)
	if len(navRow) > 0 {
		rows = append(rows, navRow)
	}

	return tgbotapi.InlineKeyboardMarkup{
		InlineKeyboard: rows,
	}
}

// buildNavRow creates the navigation row for pagination.
func buildNavRow(currentPage, totalPages int) []tgbotapi.InlineKeyboardButton {
	if totalPages <= 1 {
		return nil
	}

	navRow := make([]tgbotapi.InlineKeyboardButton, 0, 3)

	// Back button
	if currentPage > 1 {
		navRow = append(navRow, tgbotapi.InlineKeyboardButton{
			Text:         "◀️ Назад",
			CallbackData: fmt.Sprintf("wf:page:%d", currentPage-1),
		})
	} else {
		// Placeholder for alignment
		navRow = append(navRow, tgbotapi.InlineKeyboardButton{
			Text:         " ",
			CallbackData: "wf:noop",
		})
	}

	// Page indicator
	navRow = append(navRow, tgbotapi.InlineKeyboardButton{
		Text:         fmt.Sprintf("%d/%d", currentPage, totalPages),
		CallbackData: "wf:noop",
	})

	// Next button
	if currentPage < totalPages {
		navRow = append(navRow, tgbotapi.InlineKeyboardButton{
			Text:         "Далі ▶️",
			CallbackData: fmt.Sprintf("wf:page:%d", currentPage+1),
		})
	} else {
		// Placeholder for alignment
		navRow = append(navRow, tgbotapi.InlineKeyboardButton{
			Text:         " ",
			CallbackData: "wf:noop",
		})
	}

	return navRow
}

// PaginatedListWithExtra creates a paginated list with an extra button row at the bottom.
func PaginatedListWithExtra(items []SelectableItem, currentPage, totalPages int, extraButtons []tgbotapi.InlineKeyboardButton) tgbotapi.InlineKeyboardMarkup {
	keyboard := PaginatedList(items, currentPage, totalPages)

	if len(extraButtons) > 0 {
		keyboard.InlineKeyboard = append(keyboard.InlineKeyboard, extraButtons)
	}

	return keyboard
}

// GetPageSlice returns a slice of items for the given page.
func GetPageSlice[T any](items []T, page, itemsPerPage int) []T {
	if page < 1 {
		page = 1
	}

	start := (page - 1) * itemsPerPage
	if start >= len(items) {
		return nil
	}

	end := start + itemsPerPage
	if end > len(items) {
		end = len(items)
	}

	return items[start:end]
}

// CalculateTotalPages calculates the total number of pages.
func CalculateTotalPages(totalItems, itemsPerPage int) int {
	if itemsPerPage <= 0 {
		return 1
	}
	pages := totalItems / itemsPerPage
	if totalItems%itemsPerPage > 0 {
		pages++
	}
	if pages == 0 {
		pages = 1
	}
	return pages
}
