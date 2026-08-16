package server

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	var body apiError
	body.Error.Code, body.Error.Message = code, message
	writeJSON(w, status, body)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "요청 형식이 올바르지 않습니다.")
		return false
	}
	return true
}

func internalError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, r.Context().Err()) {
		return
	}
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	writeError(w, http.StatusInternalServerError, "internal_error", "요청을 처리하지 못했습니다.")
}
