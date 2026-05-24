"use client";

import React, { useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Download,
  Loader2,
  AlertCircle,
  ChevronLeft,
  ChevronRight,
  ShieldCheck,
} from "lucide-react";
import { api } from "@multica/core/api";
import { useCurrentWorkspace } from "@multica/core/paths";
import type { RdevAuditEntry } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";

const PAGE_SIZE = 25;

const AUDIT_ACTIONS = [
  "all",
  "issue.created",
  "issue.updated",
  "issue.deleted",
  "issue.status_changed",
  "comment.created",
  "agent.created",
  "agent.updated",
  "member.invited",
  "member.removed",
  "workspace.updated",
] as const;

function formatDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

function ActorBadge({ type }: { type: "member" | "agent" }) {
  return (
    <span
      className={
        type === "agent"
          ? "inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium bg-purple-100 text-purple-700 dark:bg-purple-900/40 dark:text-purple-300"
          : "inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium bg-blue-100 text-blue-700 dark:bg-blue-900/40 dark:text-blue-300"
      }
    >
      {type}
    </span>
  );
}

function entriesToCsv(entries: RdevAuditEntry[]): string {
  const header = ["ID", "Time", "Actor", "Actor Type", "Action", "Resource Type", "Resource ID", "Resource Label"];
  const rows = entries.map((e) => [
    e.id,
    e.created_at,
    e.actor_name,
    e.actor_type,
    e.action,
    e.resource_type,
    e.resource_id,
    e.resource_label ?? "",
  ]);
  return [header, ...rows]
    .map((r) => r.map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","))
    .join("\n");
}

function downloadCsv(content: string, filename: string) {
  const blob = new Blob([content], { type: "text/csv;charset=utf-8;" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

export function AuditLogPage() {
  const workspace = useCurrentWorkspace();
  const wsId = workspace?.id ?? "";

  const [page, setPage] = useState(0);
  const [since, setSince] = useState("");
  const [until, setUntil] = useState("");
  const [action, setAction] = useState("all");

  const query = useQuery({
    queryKey: ["rdev", "audit-logs", wsId, page, since, until, action],
    queryFn: () =>
      api.listAuditLogs({
        workspaceId: wsId,
        limit: PAGE_SIZE,
        offset: page * PAGE_SIZE,
        since: since || undefined,
        until: until || undefined,
        action: action !== "all" ? action : undefined,
      }),
    enabled: !!wsId,
  });

  const entries = query.data?.entries ?? [];
  const total = query.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  const handleExport = useCallback(() => {
    if (!entries.length) return;
    downloadCsv(entriesToCsv(entries), `audit-log-${new Date().toISOString().slice(0, 10)}.csv`);
  }, [entries]);

  const handleFilterChange = useCallback(() => {
    setPage(0);
  }, []);

  return (
    <div className="flex flex-col h-full">
      <div className="border-b border-border px-6 py-4">
        <div className="flex items-center gap-2">
          <ShieldCheck className="size-5 text-cyan-600" />
          <div>
            <h1 className="text-lg font-semibold">Audit Log</h1>
            <p className="text-sm text-muted-foreground">
              Track all workspace activity (admin only)
            </p>
          </div>
        </div>
      </div>

      {/* Filters */}
      <div className="border-b border-border px-6 py-3 flex flex-wrap items-end gap-4">
        <div className="flex flex-col gap-1">
          <Label className="text-xs">Since</Label>
          <Input
            type="datetime-local"
            value={since}
            onChange={(e) => { setSince(e.target.value); handleFilterChange(); }}
            className="h-8 text-xs w-44"
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label className="text-xs">Until</Label>
          <Input
            type="datetime-local"
            value={until}
            onChange={(e) => { setUntil(e.target.value); handleFilterChange(); }}
            className="h-8 text-xs w-44"
          />
        </div>
        <div className="flex flex-col gap-1">
          <Label className="text-xs">Action</Label>
          <select
            value={action}
            onChange={(e) => { setAction(e.target.value); handleFilterChange(); }}
            className="h-8 rounded-md border border-input bg-background px-2 text-xs focus:outline-none focus:ring-1 focus:ring-ring"
          >
            {AUDIT_ACTIONS.map((a) => (
              <option key={a} value={a}>{a}</option>
            ))}
          </select>
        </div>
        <Button
          variant="outline"
          size="sm"
          onClick={handleExport}
          disabled={!entries.length}
          className="h-8 gap-1.5"
        >
          <Download className="size-3.5" />
          Export CSV
        </Button>
      </div>

      {/* Table */}
      <div className="flex-1 overflow-auto">
        {query.isLoading && (
          <div className="flex items-center justify-center gap-2 py-16 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
            Loading audit log…
          </div>
        )}
        {query.isError && (
          <div className="flex items-center justify-center gap-2 py-16 text-destructive">
            <AlertCircle className="size-5" />
            Failed to load audit log
          </div>
        )}
        {!query.isLoading && !query.isError && (
          <table className="w-full text-sm">
            <thead className="sticky top-0 bg-background border-b border-border">
              <tr>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground text-xs">Time</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground text-xs">Actor</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground text-xs">Action</th>
                <th className="px-4 py-2.5 text-left font-medium text-muted-foreground text-xs">Resource</th>
              </tr>
            </thead>
            <tbody>
              {entries.length === 0 && (
                <tr>
                  <td colSpan={4} className="px-4 py-12 text-center text-muted-foreground text-sm">
                    No audit log entries found.
                  </td>
                </tr>
              )}
              {entries.map((entry) => (
                <tr key={entry.id} className="border-b border-border/60 hover:bg-muted/30">
                  <td className="px-4 py-2.5 text-xs text-muted-foreground whitespace-nowrap">
                    {formatDate(entry.created_at)}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-1.5">
                      <span className="font-medium text-xs">{entry.actor_name}</span>
                      <ActorBadge type={entry.actor_type} />
                    </div>
                  </td>
                  <td className="px-4 py-2.5">
                    <span className="font-mono text-xs bg-muted px-1.5 py-0.5 rounded">
                      {entry.action}
                    </span>
                  </td>
                  <td className="px-4 py-2.5 text-xs text-muted-foreground">
                    <span className="font-medium text-foreground">{entry.resource_type}</span>
                    {entry.resource_label && (
                      <span className="ml-1">· {entry.resource_label}</span>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Pagination */}
      <div className="border-t border-border px-6 py-2.5 flex items-center justify-between">
        <span className="text-xs text-muted-foreground">
          {total > 0 ? `${page * PAGE_SIZE + 1}–${Math.min((page + 1) * PAGE_SIZE, total)} of ${total}` : "No entries"}
        </span>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setPage((p) => Math.max(0, p - 1))}
            disabled={page === 0}
            className="h-7 w-7 p-0"
          >
            <ChevronLeft className="size-4" />
          </Button>
          <span className="text-xs text-muted-foreground px-2">
            {page + 1} / {totalPages}
          </span>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
            disabled={page >= totalPages - 1}
            className="h-7 w-7 p-0"
          >
            <ChevronRight className="size-4" />
          </Button>
        </div>
      </div>
    </div>
  );
}
