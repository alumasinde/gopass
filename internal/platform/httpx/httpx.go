package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/alumasinde/gopass/internal/platform/apperr"
)

func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes the standard error envelope. Prefer Fail with an apperr value;
// this exists for the rare case that needs an ad-hoc status/code.
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, map[string]any{"error": map[string]any{"code": code, "message": message}})
}

// Fail writes any error using its catalogue status, code and message.
func Fail(w http.ResponseWriter, err error) {
	e := apperr.From(err)
	Error(w, e.Status, e.Code, e.Message)
}

func Decode(r *http.Request, v any) error  { return json.NewDecoder(r.Body).Decode(v) }
func ID(s string) (int64, error)           { return strconv.ParseInt(s, 10, 64) }
func OK(w http.ResponseWriter, v any)      { JSON(w, http.StatusOK, map[string]any{"data": v}) }
func Created(w http.ResponseWriter, v any) { JSON(w, http.StatusCreated, map[string]any{"data": v}) }
