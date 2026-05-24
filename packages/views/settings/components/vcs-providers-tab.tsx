"use client";

import { useState } from "react";
import { Plus, Trash2, GitBranch, Link } from "lucide-react";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@multica/ui/components/ui/select";
import { toast } from "sonner";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import { memberListOptions } from "@multica/core/workspace/queries";
import { api } from "@multica/core/api";
import type { RdevVCSProvider, CreateRdevVCSProviderRequest } from "@multica/core/types";
import { useT } from "../../i18n";

const PROVIDER_LABELS: Record<string, string> = {
  gitea: "Gitea",
  github: "GitHub",
};

function AddProviderForm({
  wsId,
  onSuccess,
}: {
  wsId: string;
  onSuccess: () => void;
}) {
  const [provider, setProvider] = useState<"gitea" | "github">("gitea");
  const [baseURL, setBaseURL] = useState("");
  const [token, setToken] = useState("");
  const [displayName, setDisplayName] = useState("");

  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: (req: CreateRdevVCSProviderRequest) =>
      api.createVCSProvider(wsId, req),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["rdev", "vcs-providers", wsId] });
      toast.success("VCS provider added");
      setBaseURL("");
      setToken("");
      setDisplayName("");
      onSuccess();
    },
    onError: () => {
      toast.error("Failed to add VCS provider");
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!baseURL.trim() || !token.trim()) return;
    mutation.mutate({
      provider,
      base_url: baseURL.trim(),
      token: token.trim(),
      display_name: displayName.trim() || undefined,
    });
  };

  return (
    <form onSubmit={handleSubmit} className="border rounded-lg p-4 space-y-3 bg-muted/30">
      <h3 className="text-sm font-medium">Add VCS Provider</h3>
      <div className="grid grid-cols-2 gap-3">
        <div className="space-y-1">
          <Label className="text-xs">Provider Type</Label>
          <Select
            value={provider}
            onValueChange={(v) => setProvider(v as "gitea" | "github")}
          >
            <SelectTrigger className="h-8 text-sm">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="gitea">Gitea</SelectItem>
              <SelectItem value="github">GitHub</SelectItem>
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label className="text-xs">Display Name (optional)</Label>
          <Input
            className="h-8 text-sm"
            placeholder="My Gitea Server"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
          />
        </div>
      </div>
      <div className="space-y-1">
        <Label className="text-xs">Base URL</Label>
        <Input
          className="h-8 text-sm"
          placeholder="https://git.example.com"
          value={baseURL}
          onChange={(e) => setBaseURL(e.target.value)}
          required
        />
      </div>
      <div className="space-y-1">
        <Label className="text-xs">Personal Access Token</Label>
        <Input
          className="h-8 text-sm font-mono"
          type="password"
          placeholder="Paste token here..."
          value={token}
          onChange={(e) => setToken(e.target.value)}
          required
          autoComplete="new-password"
        />
      </div>
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          onClick={onSuccess}
        >
          Cancel
        </Button>
        <Button
          type="submit"
          size="sm"
          disabled={mutation.isPending || !baseURL.trim() || !token.trim()}
        >
          {mutation.isPending ? "Adding…" : "Add Provider"}
        </Button>
      </div>
    </form>
  );
}

function ProviderRow({
  provider,
  canManage,
  wsId,
}: {
  provider: RdevVCSProvider;
  canManage: boolean;
  wsId: string;
}) {
  const qc = useQueryClient();
  const deleteMut = useMutation({
    mutationFn: () => api.deleteVCSProvider(wsId, provider.id),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["rdev", "vcs-providers", wsId] });
      toast.success("Provider removed");
    },
    onError: () => {
      toast.error("Failed to remove provider");
    },
  });

  return (
    <div className="flex items-center gap-3 rounded-lg border px-4 py-3">
      <GitBranch className="h-4 w-4 shrink-0 text-muted-foreground" />
      <div className="flex-1 min-w-0">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium truncate">
            {provider.display_name || provider.base_url}
          </span>
          <span className="shrink-0 rounded-full bg-muted px-2 py-0.5 text-xs text-muted-foreground">
            {PROVIDER_LABELS[provider.provider] ?? provider.provider}
          </span>
        </div>
        <div className="flex items-center gap-1 mt-0.5">
          <Link className="h-3 w-3 text-muted-foreground" />
          <span className="text-xs text-muted-foreground truncate">{provider.base_url}</span>
          <span className="ml-2 font-mono text-xs text-muted-foreground">{provider.token_hint}</span>
        </div>
      </div>
      {canManage && (
        <Button
          variant="ghost"
          size="icon"
          className="h-7 w-7 shrink-0 text-muted-foreground hover:text-destructive"
          disabled={deleteMut.isPending}
          onClick={() => deleteMut.mutate()}
        >
          <Trash2 className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
}

export function VCSProvidersTab() {
  const user = useAuthStore((s) => s.user);
  const wsId = useWorkspaceId();
  const [showForm, setShowForm] = useState(false);

  const { data: members = [] } = useQuery(memberListOptions(wsId));
  const currentMember = members.find((m) => m.user_id === user?.id);
  const canManage =
    currentMember?.role === "owner" || currentMember?.role === "admin";

  const { data: providers = [], isLoading } = useQuery<RdevVCSProvider[]>({
    queryKey: ["rdev", "vcs-providers", wsId],
    queryFn: () => api.listVCSProviders(wsId),
    enabled: !!wsId,
  });

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-sm font-semibold">VCS Providers</h2>
          <p className="text-xs text-muted-foreground mt-0.5">
            Connect Gitea or GitHub instances for file browsing and code access.
          </p>
        </div>
        {canManage && !showForm && (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setShowForm(true)}
          >
            <Plus className="h-3.5 w-3.5 mr-1.5" />
            Add Provider
          </Button>
        )}
      </div>

      {showForm && (
        <AddProviderForm wsId={wsId} onSuccess={() => setShowForm(false)} />
      )}

      {isLoading ? (
        <div className="text-sm text-muted-foreground">Loading…</div>
      ) : providers.length === 0 && !showForm ? (
        <div className="rounded-lg border border-dashed p-8 text-center">
          <GitBranch className="mx-auto h-8 w-8 text-muted-foreground/40 mb-2" />
          <p className="text-sm text-muted-foreground">No VCS providers configured.</p>
          {canManage && (
            <p className="text-xs text-muted-foreground mt-1">
              Add a Gitea or GitHub provider to enable file browsing.
            </p>
          )}
        </div>
      ) : (
        <div className="space-y-2">
          {providers.map((p) => (
            <ProviderRow key={p.id} provider={p} canManage={canManage} wsId={wsId} />
          ))}
        </div>
      )}
    </div>
  );
}
