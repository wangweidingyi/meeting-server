package admin

import (
	"encoding/json"
	"net/http"
)

func NewHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/settings", func(writer http.ResponseWriter, request *http.Request) {
		setCORSHeaders(writer)
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}

		switch request.Method {
		case http.MethodGet:
			writeJSON(writer, http.StatusOK, service.Current())
		case http.MethodPut:
			var payload UpdateSettingsRequest
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				writeJSON(writer, http.StatusBadRequest, map[string]string{
					"error": "invalid_json",
				})
				return
			}

			settings, err := service.Update(request.Context(), payload)
			if err != nil {
				writeJSON(writer, http.StatusBadRequest, map[string]string{
					"error": err.Error(),
				})
				return
			}

			writeJSON(writer, http.StatusOK, settings)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/admin/settings/test", func(writer http.ResponseWriter, request *http.Request) {
		setCORSHeaders(writer)
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodPost {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		var payload UpdateSettingsRequest
		if request.Body != nil {
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil && err.Error() != "EOF" {
				writeJSON(writer, http.StatusBadRequest, map[string]string{
					"error": "invalid_json",
				})
				return
			}
		}

		result, err := service.Test(request.Context(), payload)
		if err != nil {
			writeJSON(writer, http.StatusBadRequest, map[string]string{
				"error": err.Error(),
			})
			return
		}

		writeJSON(writer, http.StatusOK, result)
	})

	mux.HandleFunc("/api/admin/users", func(writer http.ResponseWriter, request *http.Request) {
		setCORSHeaders(writer)
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		writeJSON(writer, http.StatusOK, map[string]any{
			"items": []any{},
		})
	})

	mux.HandleFunc("/api/admin/health", func(writer http.ResponseWriter, request *http.Request) {
		setCORSHeaders(writer)
		if request.Method == http.MethodOptions {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodGet {
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		writeJSON(writer, http.StatusOK, map[string]string{
			"status": "ok",
		})
	})

	return mux
}

func setCORSHeaders(writer http.ResponseWriter) {
	writer.Header().Set("Access-Control-Allow-Origin", "*")
	writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	writer.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, OPTIONS")
}

func writeJSON(writer http.ResponseWriter, status int, payload any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(payload)
}
