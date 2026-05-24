package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
)

type gatewayServer struct {
	db     *pgxpool.Pool
	router *modelRouter
	auth   *authMiddleware
}

func newServer(dbURL, routesJSON string) (http.Handler, error) {
	var pool *pgxpool.Pool
	if dbURL != "" {
		var err error
		pool, err = pgxpool.New(context.Background(), dbURL)
		if err != nil {
			return nil, fmt.Errorf("db connect: %w", err)
		}
	}

	rt, err := newModelRouter(routesJSON)
	if err != nil {
		return nil, fmt.Errorf("model router: %w", err)
	}

	gs := &gatewayServer{
		db:     pool,
		router: rt,
		auth:   newAuthMiddleware(pool),
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Route("/v1", func(r chi.Router) {
		r.Use(gs.auth.Authenticate)
		r.Post("/messages", gs.handleMessages)
	})

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	return r, nil
}

func (gs *gatewayServer) handleMessages(w http.ResponseWriter, r *http.Request) {
	var anthropicReq AnthropicRequest
	if err := json.NewDecoder(r.Body).Decode(&anthropicReq); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "invalid request body")
		return
	}

	route, err := gs.router.Route(anthropicReq.Model)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request_error", "no route for model")
		return
	}

	providerReq := translateRequest(anthropicReq, route.TargetModel)
	p := gs.router.Provider(route.Provider)

	if anthropicReq.Stream {
		flusher, ok := w.(http.Flusher)
		if !ok {
			writeJSONError(w, http.StatusInternalServerError, "api_error", "streaming not supported")
			return
		}
		events, err := p.ChatCompletionsStream(r.Context(), providerReq)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, "api_error", "upstream stream error")
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		translateStream(w, flusher, events, anthropicReq.Model)
		return
	}

	resp, err := p.ChatCompletions(r.Context(), providerReq)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "api_error", "upstream error")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(translateResponse(resp, anthropicReq.Model))
}

func writeJSONError(w http.ResponseWriter, status int, errType, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"type":    errType,
			"message": msg,
		},
	})
}
