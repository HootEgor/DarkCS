package mainmenu

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"DarkCS/bot/chat"
	"DarkCS/internal/gdrive"
	"DarkCS/internal/lib/sl"
)

const videosPerPage = 5

// SelectVideoStep shows a paginated list of training videos fetched from
// Google Drive. Users select a video to receive it; a "Back" button returns
// to the main menu. Telegram videos are sent with protect_content to prevent
// forwarding or saving. A per-step in-memory cache maps Drive file IDs to
// Telegram file_ids so the same file is not re-uploaded on every request.
type SelectVideoStep struct {
	driveService gdrive.DriveService

	mu          sync.RWMutex
	fileIDCache map[string]string // Drive file ID → Telegram file_id
}

func (s *SelectVideoStep) ID() chat.StepID { return StepSelectVideo }

func (s *SelectVideoStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	log := slog.With(slog.String("platform", state.Platform), slog.String("user_id", state.UserID))

	if s.driveService == nil {
		log.Warn("select_video: drive service not configured")
		_ = m.SendText(state.ChatID, "Навчальні матеріали тимчасово недоступні.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	videos, err := s.driveService.ListVideos()
	if err != nil {
		log.Error("select_video: list videos failed", sl.Err(err))
		_ = m.SendText(state.ChatID, "Помилка завантаження списку відео. Спробуйте пізніше.")
		return chat.StepResult{Error: err}
	}
	log.Info("select_video: videos loaded", slog.Int("count", len(videos)))

	if len(videos) == 0 {
		_ = m.SendText(state.ChatID, "Наразі відео-матеріали відсутні.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	rows := s.buildPage(videos, 0)
	if err := m.SendInlineGrid(state.ChatID, "📚 Навчальні відео\n\nОберіть відео для перегляду:", rows); err != nil {
		log.Error("select_video: send inline grid failed", sl.Err(err))
		return chat.StepResult{Error: err}
	}
	return chat.StepResult{}
}

func (s *SelectVideoStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	if s.driveService == nil {
		return chat.StepResult{NextStep: StepMainMenu}
	}

	data := input.CallbackData

	// For text-only platforms: match number input to the current page.
	if data == "" {
		videos, err := s.driveService.ListVideos()
		if err != nil || len(videos) == 0 {
			return chat.StepResult{NextStep: StepMainMenu}
		}
		page := state.GetInt("vid_page")
		rows := s.buildPage(videos, page)
		data = chat.MatchNumberToInlineGrid(input.Text, rows)
	}

	if data == "" {
		return chat.StepResult{}
	}

	// Back to main menu.
	if data == "vid_back" {
		return chat.StepResult{NextStep: StepMainMenu}
	}

	// Pagination — edit the existing message in place.
	if strings.HasPrefix(data, "vid_pg:") {
		pageStr := strings.TrimPrefix(data, "vid_pg:")
		if pageStr == "noop" {
			return chat.StepResult{}
		}
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return chat.StepResult{}
		}
		videos, err := s.driveService.ListVideos()
		if err != nil || len(videos) == 0 {
			return chat.StepResult{NextStep: StepMainMenu}
		}
		rows := s.buildPage(videos, page)
		if input.MessageID != "" {
			_ = m.EditInlineGrid(state.ChatID, input.MessageID, "📚 Навчальні відео\n\nОберіть відео для перегляду:", rows)
		} else {
			_ = m.SendInlineGrid(state.ChatID, "📚 Навчальні відео\n\nОберіть відео для перегляду:", rows)
		}
		return chat.StepResult{
			UpdateState: map[string]any{"vid_page": page},
		}
	}

	// Video selection.
	if strings.HasPrefix(data, "vid_sel:") {
		idxStr := strings.TrimPrefix(data, "vid_sel:")
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			return chat.StepResult{}
		}
		videos, err := s.driveService.ListVideos()
		if err != nil || idx < 0 || idx >= len(videos) {
			_ = m.SendText(state.ChatID, "Помилка завантаження відео. Спробуйте пізніше.")
			return chat.StepResult{}
		}
		video := videos[idx]

		s.mu.RLock()
		cached := s.fileIDCache[video.ID]
		s.mu.RUnlock()

		// Download from Drive only when there is no cached Telegram file_id.
		var reader io.Reader
		if cached == "" {
			rc, dlErr := s.driveService.DownloadVideo(video.ID)
			if dlErr != nil {
				_ = m.SendText(state.ChatID, "Помилка завантаження відео. Спробуйте пізніше.")
				return chat.StepResult{}
			}
			defer rc.Close()
			reader = rc
		}

		returnedID, sendErr := m.SendVideo(
			state.ChatID,
			reader,
			cached,
			video.WebContentLink,
			video.Name,
			true, // protect_content on Telegram
		)
		if sendErr != nil {
			_ = m.SendText(state.ChatID, "Помилка відправки відео. Спробуйте пізніше.")
			return chat.StepResult{}
		}

		if returnedID != "" {
			s.mu.Lock()
			s.fileIDCache[video.ID] = returnedID
			s.mu.Unlock()
		}

		return chat.StepResult{}
	}

	return chat.StepResult{}
}

// buildPage constructs a page of inline video buttons plus navigation and a
// "Back to menu" row. Page numbering is zero-based.
func (s *SelectVideoStep) buildPage(videos []gdrive.VideoItem, page int) [][]chat.InlineButton {
	start := page * videosPerPage
	if start >= len(videos) {
		start = 0
		page = 0
	}
	end := start + videosPerPage
	if end > len(videos) {
		end = len(videos)
	}

	var rows [][]chat.InlineButton
	for i, v := range videos[start:end] {
		rows = append(rows, []chat.InlineButton{
			{Text: "📹 " + v.Name, Data: fmt.Sprintf("vid_sel:%d", start+i)},
		})
	}

	// Navigation row (shown only when there is more than one page).
	totalPages := (len(videos) + videosPerPage - 1) / videosPerPage
	if totalPages > 1 {
		backData := "vid_pg:noop"
		if page > 0 {
			backData = fmt.Sprintf("vid_pg:%d", page-1)
		}
		fwdData := "vid_pg:noop"
		if page < totalPages-1 {
			fwdData = fmt.Sprintf("vid_pg:%d", page+1)
		}
		rows = append(rows, []chat.InlineButton{
			{Text: "⬅️", Data: backData},
			{Text: fmt.Sprintf("%d/%d", page+1, totalPages), Data: "vid_pg:noop"},
			{Text: "➡️", Data: fwdData},
		})
	}

	rows = append(rows, []chat.InlineButton{
		{Text: BtnBack + " до меню", Data: "vid_back"},
	})

	return rows
}
