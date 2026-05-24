export interface RdevGatewayModel {
  id: string;
  label: string;
  provider: string;
  provider_type: "vllm" | "ollama" | string;
}

export type RdevRepoTreeEntryType = "blob" | "tree";

export interface RdevRepoTreeEntry {
  name: string;
  path: string;
  type: RdevRepoTreeEntryType;
  size?: number;
}

export type RdevAuditAction =
  | "issue.created"
  | "issue.updated"
  | "issue.deleted"
  | "issue.status_changed"
  | "issue.assigned"
  | "comment.created"
  | "comment.deleted"
  | "agent.created"
  | "agent.updated"
  | "agent.deleted"
  | "member.invited"
  | "member.removed"
  | "member.role_changed"
  | "workspace.updated"
  | string;

export interface RdevVCSProvider {
  id: string;
  workspace_id: string;
  provider: "gitea" | "github";
  base_url: string;
  display_name?: string;
  token_hint: string;
  created_at: string;
}

export interface CreateRdevVCSProviderRequest {
  provider: "gitea" | "github";
  base_url: string;
  token: string;
  display_name?: string;
}

export interface RdevAuditEntry {
  id: string;
  workspace_id: string;
  actor_id: string;
  actor_name: string;
  actor_type: "member" | "agent";
  action: RdevAuditAction;
  resource_type: string;
  resource_id: string;
  resource_label?: string;
  created_at: string;
}
