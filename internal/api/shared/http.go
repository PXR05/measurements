package shared

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type errorResponse struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Responder struct{ Logger *log.Logger }

func (a *Responder) WriteJSON(w http.ResponseWriter, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		a.Logger.Printf("encode JSON: %v", err)
		status = http.StatusInternalServerError
		data = []byte(`{"error":{"code":"internal_error","message":"internal server error"}}`)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(data, '\n'))
}

func (a *Responder) WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "internal server error"

	var input *RequestError
	switch {
	case errors.As(err, &input):
		status, message = input.Status, input.Message
		code = "invalid_request"
		if status == http.StatusMethodNotAllowed {
			code = "method_not_allowed"
		}
	case errors.Is(err, sql.ErrNoRows):
		status = http.StatusNotFound
		code = "not_found"
		message = "record or endpoint not found"
	default:
		a.Logger.Printf("%s %s: %v", r.Method, r.URL.Path, err)
	}

	a.WriteJSON(w, status, errorResponse{Error: errorDetail{Code: code, Message: message}})
}
