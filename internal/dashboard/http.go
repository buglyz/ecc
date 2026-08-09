package dashboard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func (s *Server) writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(value); err != nil {
		s.logger.Printf("响应 JSON 编码失败: %v", err)
	}
}

func methodNotAllowed(w http.ResponseWriter) {
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

const maxRequestBody = 64 * 1024

var errRequestBodyTooLarge = errors.New("request body exceeds 64 KiB")

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(v); err != nil {
		return jsonDecodeError(err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request body must contain a single JSON value")
		}
		return jsonDecodeError(err)
	}
	return nil
}

func jsonDecodeError(err error) error {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		return errRequestBodyTooLarge
	}
	return err
}

func writeJSONDecodeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, errRequestBodyTooLarge) {
		status = http.StatusRequestEntityTooLarge
	}
	http.Error(w, err.Error(), status)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}