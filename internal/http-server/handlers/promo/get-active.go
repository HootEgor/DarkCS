package promo

import (
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/xuri/excelize/v2"
	"log/slog"
	"net/http"
)

func GetActivePromoCodes(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.promo")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("promo service not available")
			http.Error(w, "Promo service not available", http.StatusServiceUnavailable)
			return
		}

		codes, err := handler.GetActivePromoCodes()
		if err != nil {
			logger.Error("failed to fetch active promo codes", sl.Err(err))
			http.Error(w, fmt.Sprintf("Failed to fetch codes: %v", err), http.StatusInternalServerError)
			return
		}

		// ✅ Create Excel file
		f := excelize.NewFile()
		sheet := "PromoCodes"
		f.NewSheet(sheet)
		f.SetCellValue(sheet, "A1", "Promo Code")

		for i, code := range codes {
			cell := fmt.Sprintf("A%d", i+2)        // Start from row 2
			f.SetCellValue(sheet, cell, code.Code) // Assuming codes are entity.PromoCode
		}

		// ✅ Write Excel file to response
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		w.Header().Set("Content-Disposition", `attachment; filename="promo_codes.xlsx"`)
		w.WriteHeader(http.StatusOK)
		if err := f.Write(w); err != nil {
			logger.Error("failed to write excel file", sl.Err(err))
			http.Error(w, "Failed to generate Excel", http.StatusInternalServerError)
			return
		}

		logger.Info("excel file with promo codes sent successfully")
	}
}
