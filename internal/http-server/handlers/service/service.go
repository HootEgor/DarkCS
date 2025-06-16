package service

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/api/cont"
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type Service interface {
	SendMail(message *entity.MailMessage) (interface{}, error)
	SendEvent(message *entity.EventMessage) (interface{}, error)
}

func SendTestMail(logger *slog.Logger, handler Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		user := cont.GetUser(r.Context())

		log := logger.With(
			sl.Module("handlers.service"),
			slog.String("user", user.Username),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var message entity.MailMessage
		if err := render.Bind(r, &message); err != nil {
			log.Error("bind test mail message", sl.Err(err))
			render.Status(r, 400)
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to decode: %v", err)))
			return
		}

		log = log.With(
			slog.String("message.to", message.To),
			sl.Secret("message", message.Message),
		)
		message.Sender = user

		data, err := handler.SendMail(&message)
		if err != nil {
			log.Error("send test mail message", sl.Err(err))
			render.Status(r, 204)
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to send test mail message: %v", err)))
			return
		}
		log.Info("test mail message sent")

		render.JSON(w, r, response.Ok(data))
	}
}

func SendTestEvent(logger *slog.Logger, handler Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		user := cont.GetUser(r.Context())

		log := logger.With(
			sl.Module("handlers.service"),
			slog.String("user", user.Username),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		var message entity.EventMessage
		if err := render.Bind(r, &message); err != nil {
			log.Error("bind test event message", sl.Err(err))
			render.Status(r, 400)
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to decode: %v", err)))
			return
		}

		log = log.With(
			slog.String("message.type", message.Type),
			sl.Secret("message.username", message.Username),
		)
		message.Sender = user

		data, err := handler.SendEvent(&message)
		if err != nil {
			log.Error("send test event message", sl.Err(err))
			render.Status(r, 204)
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to send test event message: %v", err)))
			return
		}
		log.Info("test event message sent")

		render.JSON(w, r, response.Ok(data))
	}
}
