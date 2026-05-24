// @vitest-environment jsdom

import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { RdevGatewayModel } from "@multica/core/types";

const mockModels: RdevGatewayModel[] = [
  { id: "llama3:8b", label: "LLaMA 3 8B", provider: "ollama-local", provider_type: "ollama" },
  { id: "mistral:7b", label: "Mistral 7B", provider: "vllm-cluster", provider_type: "vllm" },
];

vi.mock("@multica/core/api", () => ({
  api: {
    listGatewayModels: vi.fn(),
  },
}));

vi.mock("@multica/ui/components/ui/popover", () => ({
  Popover: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  PopoverTrigger: ({ children, disabled, className, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { children: React.ReactNode }) => (
    <button type="button" disabled={disabled} className={className} {...props}>{children}</button>
  ),
  PopoverContent: ({ children }: { children: React.ReactNode }) => <div data-testid="popover-content">{children}</div>,
}));

import { GatewayModelPicker } from "./gateway-model-picker";
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

describe("GatewayModelPicker", () => {
  beforeEach(() => {
    vi.mocked(api.listGatewayModels).mockResolvedValue(mockModels);
  });

  it("shows placeholder when no value selected", async () => {
    render(
      <Wrapper>
        <GatewayModelPicker value="" onChange={vi.fn()} />
      </Wrapper>,
    );
    expect(screen.getByText("Select gateway model…")).toBeInTheDocument();
  });

  it("shows selected model label", async () => {
    render(
      <Wrapper>
        <GatewayModelPicker value="llama3:8b" onChange={vi.fn()} />
      </Wrapper>,
    );
    expect(screen.getByText("LLaMA 3 8B")).toBeInTheDocument();
  });

  it("lists models in popover grouped by provider type", async () => {
    render(
      <Wrapper>
        <GatewayModelPicker value="" onChange={vi.fn()} />
      </Wrapper>,
    );
    await waitFor(() => {
      expect(screen.getByText("LLaMA 3 8B")).toBeInTheDocument();
      expect(screen.getByText("Mistral 7B")).toBeInTheDocument();
    });
    expect(screen.getByText("ollama")).toBeInTheDocument();
    expect(screen.getByText("vllm")).toBeInTheDocument();
  });

  it("calls onChange when a model is selected", async () => {
    const onChange = vi.fn();
    render(
      <Wrapper>
        <GatewayModelPicker value="" onChange={onChange} />
      </Wrapper>,
    );
    await waitFor(() => expect(screen.getByText("LLaMA 3 8B")).toBeInTheDocument());
    await userEvent.click(screen.getByText("LLaMA 3 8B"));
    expect(onChange).toHaveBeenCalledWith("llama3:8b");
  });

  it("is disabled when disabled prop is set", () => {
    render(
      <Wrapper>
        <GatewayModelPicker value="" onChange={vi.fn()} disabled />
      </Wrapper>,
    );
    const trigger = screen.getByRole("button");
    expect(trigger).toBeDisabled();
  });
});
