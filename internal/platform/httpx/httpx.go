package httpx

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func JSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}
func Error(w http.ResponseWriter, s, c, m string) {
	JSON(w, s, map[string]any{"error": map[string]any{"code": c, "message": m}})
}
func Decode(r *http.Request, v any) error  { return json.NewDecoder(r.Body).Decode(v) }
func ID(s string) (int64, error)           { return strconv.ParseInt(s, 10, 64) }
func OK(w http.ResponseWriter, v any)      { JSON(w, 200, map[string]any{"data": v}) }
func Created(w http.ResponseWriter, v any) { JSON(w, 201, map[string]any{"data": v}) }
