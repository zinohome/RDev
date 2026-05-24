// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RdevRepoTreeEntry } from "@multica/core/types";

const mockTree: RdevRepoTreeEntry[] = [
  { name: "src", path: "src", type: "tree" },
  { name: "README.md", path: "README.md", type: "blob", size: 1024 },
  { name: "package.json", path: "package.json", type: "blob", size: 512 },
];

vi.mock("@multica/core/api", () => ({
  api: {
    listRepoTree: vi.fn(),
    getRepoFile: vi.fn(),
  },
}));

vi.mock("@multica/core/hooks", () => ({
  useWorkspaceId: () => "ws-1",
}));

vi.mock("@multica/ui/components/ui/button", () => ({
  Button: ({ children, onClick, disabled, className }: React.ButtonHTMLAttributes<HTMLButtonElement> & { children: React.ReactNode }) => (
    <button type="button" onClick={onClick} disabled={disabled} className={className}>{children}</button>
  ),
}));

import { FileBrowserPage, ReposPage } from "./file-browser-page";
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

describe("FileBrowserPage", () => {
  beforeEach(() => {
    vi.mocked(api.listRepoTree).mockResolvedValue(mockTree);
    vi.mocked(api.getRepoFile).mockResolvedValue({ content: "console.log('hello')", encoding: "utf-8" });
  });

  it("shows empty state when no repo selected", () => {
    render(<Wrapper><FileBrowserPage /></Wrapper>);
    expect(screen.getByText("Select a repository")).toBeInTheDocument();
  });

  it("shows repo name in header when repo is provided", async () => {
    render(
      <Wrapper>
        <FileBrowserPage
          initialProviderId="github"
          initialOwner="acme"
          initialRepo="frontend"
        />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText("acme/frontend")).toBeInTheDocument();
    });
  });

  it("displays file tree entries", async () => {
    render(
      <Wrapper>
        <FileBrowserPage
          initialProviderId="github"
          initialOwner="acme"
          initialRepo="frontend"
        />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText("src")).toBeInTheDocument();
      expect(screen.getByText("README.md")).toBeInTheDocument();
      expect(screen.getByText("package.json")).toBeInTheDocument();
    });
  });

  it("shows placeholder when no file selected", async () => {
    render(
      <Wrapper>
        <FileBrowserPage
          initialProviderId="github"
          initialOwner="acme"
          initialRepo="frontend"
        />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText("Select a file to view its contents")).toBeInTheDocument();
    });
  });
});

describe("ReposPage", () => {
  it("renders page heading", () => {
    render(<Wrapper><ReposPage /></Wrapper>);
    expect(screen.getByText("Repositories")).toBeInTheDocument();
  });
});
