package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newServer(db *pgxpool.Pool, mr *ModelRouter) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(middleware.RequestID)

	auth := newAuthMiddleware(db)

	r.Group(func(r chi.Router) {
		r.Use(auth.Handler)
		r.Post("/v1/messages", handleMessages(mr))
	})

	// Health check — no auth required.
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return r
}

func handleMessages(mr *ModelRouter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request_error", "failed to read request body")
			return
		}

		var ar AnthropicRequest
		if err := json.Unmarshal(body, &ar); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request_error", "malformed request JSON")
			return
		}

		match, err := mr.Route(ar.Model)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}

		oaiReq, err := translateRequest(&ar, match.TargetModel)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "api_error", "request translation failed: "+err.Error())
			return
		}

		oaiBody, err := json.Marshal(oaiReq)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "api_error", "failed to encode upstream request")
			return
		}

		resp, err := match.Provider.Chat(r.Context(), match.TargetModel, oaiBody, ar.Stream)
		if err != nil {
			log.Printf("upstream error: %v", err)
			writeError(w, http.StatusBadGateway, "api_error", "upstream provider error")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			upstream, _ := io.ReadAll(resp.Body)
			log.Printf("upstream non-200 (%d): %s", resp.StatusCode, upstream)
			writeError(w, http.StatusBadGateway, "api_error", "upstream returned non-200")
			return
		}

		if ar.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.WriteHeader(http.StatusOK)

			if err := streamResponse(w, resp.Body, ar.Model); err != nil {
				log.Printf("stream error: %v", err)
			}
			return
		}

		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			writeError(w, http.StatusBadGateway, "api_error", "failed to read upstream response")
			return
		}

		translated, err := translateNonStreamResponse(respBody, ar.Model)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "api_error", "response translation failed: "+err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(translated)
	}
}

func writeError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": msg,
		},
	})
}
