package httpserver

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"task-service/internal/domain"

	"github.com/go-chi/chi/v5/middleware"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func writeErr(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	status, message := statusOf(err)
	if status >= http.StatusInternalServerError {
		logger.Error("request failed", "request_id", middleware.GetReqID(r.Context()), "err", err)
		message = "внутренняя ошибка"
	}
	writeJSON(w, status, map[string]string{"error": message})
}

func statusOf(err error) (int, string) {
	var invalid domain.InvalidError
	var maxBytes *http.MaxBytesError
	switch {
	case errors.As(err, &invalid):
		return http.StatusBadRequest, invalid.Msg
	case errors.As(err, &maxBytes):
		return http.StatusBadRequest, "слишком большое тело запроса"
	case errors.Is(err, domain.ErrInvalid):
		return http.StatusBadRequest, "некорректный запрос"
	case errors.Is(err, domain.ErrUnauthorized):
		return http.StatusUnauthorized, "требуется аутентификация"
	case errors.Is(err, domain.ErrForbidden):
		return http.StatusForbidden, "недостаточно прав"
	case errors.Is(err, domain.ErrNotFound):
		return http.StatusNotFound, "не найдено"
	case errors.Is(err, domain.ErrConflict):
		return http.StatusConflict, "конфликт данных"
	default:
		return http.StatusInternalServerError, "внутренняя ошибка"
	}
}

func decode(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			return err
		}
		return domain.Invalid("некорректное тело запроса")
	}
	return nil
}
