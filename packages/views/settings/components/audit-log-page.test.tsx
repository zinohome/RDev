// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RdevAuditEntry } from "@multica/core/types";

const mockEntries: RdevAuditEntry[] = [
  {
    id: "e1",
    workspace_id: "ws-1",
    actor_id: "user-1",
    actor_name: "Alice",
    actor_type: "member",
    action: "issue.created",
    resource_type: "issue",
    resource_id: "issue-1",
    resource_label: "Fix login bug",
    created_at: "2026-05-24T10:00:00Z",
  },
  {
    id: "e2",
    workspace_id: "ws-1",
    actor_id: "agent-1",
    actor_name: "Dev Bot",
    actor_type: "agent",
    action: "comment.created",
    resource_type: "comment",
    resource_id: "comment-1",
    resource_label: undefined,
    created_at: "2026-05-24T11:00:00Z",
  },
];

vi.mock("@multica/core/api", () => ({
  api: {
    listAuditLogs: vi.fn(),
  },
}));

vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Test Workspace", slug: "test" }),
}));

vi.mock("@multica/ui/components/ui/button", () => ({
  Button: ({ children, onClick, disabled, className }: React.ButtonHTMLAttributes<HTMLButtonElement> & { children: React.ReactNode }) => (
    <button type="button" onClick={onClick} disabled={disabled} className={className}>{children}</button>
  ),
}));

import { AuditLogPage } from "./audit-log-page";
import { api } from "@multica/core/api";

function makeClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

function Wrapper({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={makeClient()}>
      {children}
    </QueryClientProvider>
  );
}

describe("AuditLogPage", () => {
  beforeEach(() => {
    vi.mocked(api.listAuditLogs).mockResolvedValue({ entries: mockEntries, total: 2 });
  });

  it("renders page title", () => {
    render(<Wrapper><AuditLogPage /></Wrapper>);
    expect(screen.getByText("Audit Log")).toBeInTheDocument();
  });

  it("displays audit log entries", async () => {
    render(<Wrapper><AuditLogPage /></Wrapper>);
    await waitFor(() => {
      expect(screen.getByText("Alice")).toBeInTheDocument();
      expect(screen.getByText("Dev Bot")).toBeInTheDocument();
    });
    expect(screen.getByText("issue.created")).toBeInTheDocument();
    expect(screen.getByText("comment.created")).toBeInTheDocument();
  });

  it("shows actor type badges", async () => {
    render(<Wrapper><AuditLogPage /></Wrapper>);
    await waitFor(() => {
      expect(screen.getByText("member")).toBeInTheDocument();
      expect(screen.getByText("agent")).toBeInTheDocument();
    });
  });

  it("shows resource label when present", async () => {
    render(<Wrapper><AuditLogPage /></Wrapper>);
    await waitFor(() => {
      expect(screen.getByText(/Fix login bug/)).toBeInTheDocument();
    });
  });

  it("shows total count in pagination", async () => {
    render(<Wrapper><AuditLogPage /></Wrapper>);
    await waitFor(() => {
      expect(screen.getByText(/1–2 of 2/)).toBeInTheDocument();
    });
  });

  it("shows empty state when no entries", async () => {
    vi.mocked(api.listAuditLogs).mockResolvedValue({ entries: [], total: 0 });
    render(<Wrapper><AuditLogPage /></Wrapper>);
    await waitFor(() => {
      expect(screen.getByText("No audit log entries found.")).toBeInTheDocument();
    });
  });
});
