package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zinohome/RDev/rdev/gateway/provider"
)

type RouteConfig struct {
	Routes    []RouteRule                `json:"routes"`
	Providers map[string]ProviderConfig  `json:"providers"`
}

type RouteRule struct {
	ModelPrefix string `json:"model_prefix"`
	Provider    string `json:"provider"`
	TargetModel string `json:"target_model"`
}

type ProviderConfig struct {
	BaseURL string `json:"base_url"`
}

type RouteMatch struct {
	Provider    string
	TargetModel string
}

type modelRouter struct {
	rules     []RouteRule
	providers map[string]provider.LLMProvider
}

func newModelRouter(routesJSON string) (*modelRouter, error) {
	cfg := RouteConfig{
		Routes: []RouteRule{
			{ModelPrefix: "*", Provider: "vllm", TargetModel: "Qwen/Qwen2.5-Coder-32B-Instruct"},
		},
		Providers: map[string]ProviderConfig{
			"vllm":   {BaseURL: "http://vllm:8000"},
			"ollama": {BaseURL: "http://ollama:11434"},
		},
	}

	if routesJSON != "" {
		if err := json.Unmarshal([]byte(routesJSON), &cfg); err != nil {
			return nil, fmt.Errorf("parse routes config: %w", err)
		}
	}

	providers := map[string]provider.LLMProvider{}
	for name, pc := range cfg.Providers {
		switch name {
		case "ollama":
			providers[name] = provider.NewOllamaProvider(pc.BaseURL)
		default:
			providers[name] = provider.NewVLLMProvider(pc.BaseURL)
		}
	}

	return &modelRouter{rules: cfg.Routes, providers: providers}, nil
}

func (r *modelRouter) Route(model string) (RouteMatch, error) {
	var fallback *RouteRule

	for i := range r.rules {
		rule := &r.rules[i]
		if rule.ModelPrefix == "*" {
			fallback = rule
			continue
		}
		if strings.HasPrefix(model, rule.ModelPrefix) {
			return RouteMatch{Provider: rule.Provider, TargetModel: rule.TargetModel}, nil
		}
	}

	if fallback != nil {
		return RouteMatch{Provider: fallback.Provider, TargetModel: fallback.TargetModel}, nil
	}

	return RouteMatch{}, fmt.Errorf("no route for model %q", model)
}

func (r *modelRouter) Provider(name string) provider.LLMProvider {
	p, ok := r.providers[name]
	if !ok {
		// return first available
		for _, v := range r.providers {
			return v
		}
	}
	return p
}
