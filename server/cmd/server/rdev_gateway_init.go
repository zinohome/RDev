// rdev_gateway_init.go exposes GET /api/rdev/gateway/models so the frontend model picker
// can list available LLM models configured in the gateway.
// Reads RDEV_GATEWAY_ROUTES env (same JSON consumed by the gateway service).
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/multica-ai/multica/server/internal/extension"
)

type gatewayModelEntry struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Provider     string `json:"provider"`
	ProviderType string `json:"provider_type"`
}

type gatewayRouteConfig struct {
	Routes    []gatewayRoute              `json:"routes"`
	Providers map[string]gatewayProvider  `json:"providers"`
}

type gatewayRoute struct {
	ModelPrefix string `json:"model_prefix"`
	Provider    string `json:"provider"`
	TargetModel string `json:"target_model"`
}

type gatewayProvider struct {
	BaseURL string `json:"base_url"`
}

func loadGatewayModels() []gatewayModelEntry {
	cfg := gatewayRouteConfig{
		Routes: []gatewayRoute{
			{ModelPrefix: "*", Provider: "vllm", TargetModel: "Qwen/Qwen2.5-Coder-32B-Instruct"},
		},
		Providers: map[string]gatewayProvider{
			"vllm":   {BaseURL: "http://vllm:8000"},
			"ollama": {BaseURL: "http://ollama:11434"},
		},
	}

	if raw := strings.TrimSpace(os.Getenv("RDEV_GATEWAY_ROUTES")); raw != "" {
		_ = json.Unmarshal([]byte(raw), &cfg)
	}

	var models []gatewayModelEntry
	for _, route := range cfg.Routes {
		if route.ModelPrefix == "*" || route.TargetModel == "" {
			continue
		}
		providerType := route.Provider
		models = append(models, gatewayModelEntry{
			ID:           route.TargetModel,
			Label:        route.TargetModel,
			Provider:     route.Provider,
			ProviderType: providerType,
		})
	}

	// Always include the fallback/default model if no explicit routes exist.
	for _, route := range cfg.Routes {
		if route.ModelPrefix == "*" && route.TargetModel != "" {
			models = append(models, gatewayModelEntry{
				ID:           route.TargetModel,
				Label:        route.TargetModel + " (default)",
				Provider:     route.Provider,
				ProviderType: route.Provider,
			})
			break
		}
	}

	if len(models) == 0 {
		models = []gatewayModelEntry{
			{ID: "Qwen/Qwen2.5-Coder-32B-Instruct", Label: "Qwen2.5-Coder-32B (vLLM)", Provider: "vllm", ProviderType: "vllm"},
		}
	}
	return models
}

func handleGatewayModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loadGatewayModels())
}

func init() {
	extension.RegisterExtensionRoutes(func(r chi.Router) {
		r.Get("/api/rdev/gateway/models", handleGatewayModels)
	})
}
