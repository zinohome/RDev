"use client";

import { useState, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { ChevronDown, Cpu, Loader2, Check, AlertCircle } from "lucide-react";
import { api } from "@multica/core/api";
import type { RdevGatewayModel } from "@multica/core/types";
import {
  Popover,
  PopoverTrigger,
  PopoverContent,
} from "@multica/ui/components/ui/popover";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";

const GATEWAY_MODELS_QUERY_KEY = ["rdev", "gateway", "models"] as const;

function useGatewayModels() {
  return useQuery({
    queryKey: GATEWAY_MODELS_QUERY_KEY,
    queryFn: () => api.listGatewayModels(),
    staleTime: 60_000,
  });
}

function groupByProviderType(models: RdevGatewayModel[]): Record<string, RdevGatewayModel[]> {
  const out: Record<string, RdevGatewayModel[]> = {};
  for (const m of models) {
    const key = m.provider_type || m.provider || "";
    if (!out[key]) out[key] = [];
    out[key].push(m);
  }
  return out;
}

export function GatewayModelPicker({
  value,
  onChange,
  disabled,
}: {
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const { data, isLoading, isError } = useGatewayModels();
  const models = useMemo(() => data ?? [], [data]);

  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = q
      ? models.filter(
          (m) =>
            m.id.toLowerCase().includes(q) ||
            m.label.toLowerCase().includes(q) ||
            m.provider.toLowerCase().includes(q),
        )
      : models;
    return groupByProviderType(list);
  }, [models, search]);

  const selectedModel = models.find((m) => m.id === value);
  const triggerLabel = selectedModel?.label ?? value || "Select gateway model…";

  const select = (id: string) => {
    onChange(id);
    setOpen(false);
    setSearch("");
  };

  return (
    <div className="flex flex-col min-w-0">
      <div className="flex h-6 items-center justify-between">
        <Label className="text-xs text-muted-foreground">Gateway Model</Label>
        {isError && (
          <span className="flex items-center gap-1 text-xs text-destructive">
            <AlertCircle className="size-3" />
            Failed to load
          </span>
        )}
      </div>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger
          disabled={disabled}
          className="flex w-full min-w-0 items-center gap-3 rounded-lg border border-border bg-background px-3 py-2.5 mt-1.5 text-left text-sm transition-colors hover:bg-muted disabled:pointer-events-none disabled:opacity-50"
        >
          <Cpu className="h-4 w-4 shrink-0 text-cyan-600" />
          <div className="min-w-0 flex-1">
            <span className="truncate font-medium">{triggerLabel}</span>
            {selectedModel && (
              <div className="truncate text-xs text-muted-foreground">
                {selectedModel.provider} · {selectedModel.provider_type}
              </div>
            )}
          </div>
          <ChevronDown
            className={`h-4 w-4 shrink-0 text-muted-foreground transition-transform ${open ? "rotate-180" : ""}`}
          />
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="w-[var(--anchor-width)] p-0 overflow-hidden"
        >
          <div className="border-b border-border p-2">
            <Input
              autoFocus
              placeholder="Search models…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-8"
            />
          </div>
          <div className="max-h-72 overflow-y-auto p-1">
            {isLoading && (
              <div className="flex items-center gap-2 px-3 py-6 text-sm text-muted-foreground">
                <Loader2 className="h-4 w-4 animate-spin" />
                Discovering models…
              </div>
            )}

            {!isLoading &&
              Object.entries(filtered).map(([providerType, list]) => (
                <div key={providerType} className="mb-1">
                  {providerType && (
                    <div className="px-2 pt-1.5 pb-0.5 text-xs font-medium uppercase tracking-wide text-muted-foreground">
                      {providerType}
                    </div>
                  )}
                  {list.map((m) => (
                    <button
                      key={m.id}
                      type="button"
                      onClick={() => select(m.id)}
                      className={`flex w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-colors ${
                        m.id === value ? "bg-accent" : "hover:bg-accent/50"
                      }`}
                    >
                      <div className="min-w-0 flex-1">
                        <div className="truncate font-medium">{m.label}</div>
                        <div className="truncate text-xs text-muted-foreground">
                          {m.provider} · {m.id}
                        </div>
                      </div>
                      {m.id === value && (
                        <Check className="h-4 w-4 shrink-0 text-primary" />
                      )}
                    </button>
                  ))}
                </div>
              ))}

            {!isLoading && Object.keys(filtered).length === 0 && (
              <div className="px-3 py-6 text-center text-sm text-muted-foreground">
                No gateway models found.
              </div>
            )}

            {value && (
              <button
                type="button"
                onClick={() => select("")}
                className="mt-1 flex w-full items-center gap-2 border-t border-border px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:bg-accent/50"
              >
                Clear selection
              </button>
            )}
          </div>
        </PopoverContent>
      </Popover>
    </div>
  );
}
