package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/zinohome/RDev/rdev/gateway/provider"
)

// RoutesConfig is the JSON structure of RDEV_GATEWAY_ROUTES.
type RoutesConfig struct {
	Routes    []RouteEntry                 `json:"routes"`
	Providers map[string]ProviderConfigRaw `json:"providers"`
}

type RouteEntry struct {
	ModelPrefix string `json:"model_prefix"`
	Provider    string `json:"provider"`
	TargetModel string `json:"target_model"`
}

type ProviderConfigRaw struct {
	BaseURL string `json:"base_url"`
}

// RouteMatch is the result of a routing decision.
type RouteMatch struct {
	TargetModel string
	Provider    provider.LLMProvider
}

// ModelRouter resolves a Claude model ID to a backend provider and model name.
type ModelRouter struct {
	routes    []RouteEntry
	providers map[string]provider.LLMProvider
}

func NewModelRouter() (*ModelRouter, error) {
	raw := os.Getenv("RDEV_GATEWAY_ROUTES")
	if raw == "" {
		log.Println("warn: RDEV_GATEWAY_ROUTES not set, running in passthrough mode (all routing will fail)")
		return &ModelRouter{routes: nil, providers: nil}, nil
	}
	var cfg RoutesConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return nil, fmt.Errorf("parse RDEV_GATEWAY_ROUTES: %w", err)
	}

	providers := make(map[string]provider.LLMProvider, len(cfg.Providers))
	for name, pcfg := range cfg.Providers {
		switch name {
		case "vllm":
			providers[name] = provider.NewVLLM(provider.Config{BaseURL: pcfg.BaseURL})
		case "ollama":
			providers[name] = provider.NewOllama(provider.Config{BaseURL: pcfg.BaseURL})
		default:
			return nil, fmt.Errorf("unknown provider %q", name)
		}
	}

	return &ModelRouter{routes: cfg.Routes, providers: providers}, nil
}

// Route returns the provider and target model for the given Claude model ID.
// Matching order: exact prefix → wildcard "*".
func (r *ModelRouter) Route(claudeModel string) (RouteMatch, error) {
	var wildcard *RouteEntry
	for i := range r.routes {
		entry := &r.routes[i]
		if entry.ModelPrefix == "*" {
			wildcard = entry
			continue
		}
		if strings.HasPrefix(claudeModel, entry.ModelPrefix) {
			return r.resolve(entry)
		}
	}
	if wildcard != nil {
		return r.resolve(wildcard)
	}
	return RouteMatch{}, fmt.Errorf("no route for model %q", claudeModel)
}

func (r *ModelRouter) resolve(entry *RouteEntry) (RouteMatch, error) {
	p, ok := r.providers[entry.Provider]
	if !ok {
		return RouteMatch{}, fmt.Errorf("provider %q not configured", entry.Provider)
	}
	return RouteMatch{TargetModel: entry.TargetModel, Provider: p}, nil
}
