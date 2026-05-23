// File browser API client for rdev/files HTTP endpoints.

const BASE = "/api/rdev/files";

export interface TreeEntry {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  mod_time?: string;
}

export interface ReadResult {
  content?: string;
  encoding: "utf-8" | "binary";
  truncated: boolean;
}

export interface DiffResult {
  patch: string;
}

export type SourceKind = "vcs" | "runtime";

export interface VCSSource {
  kind: "vcs";
  providerID: string;
  owner: string;
  repo: string;
  branch: string;
}

export interface RuntimeSource {
  kind: "runtime";
  runtimeID: string;
  taskID: string;
}

export type FileSource = VCSSource | RuntimeSource;

export function encodeSource(s: FileSource): string {
  if (s.kind === "vcs") {
    return `vcs:${s.providerID}:${s.owner}/${s.repo}:${s.branch}`;
  }
  return `runtime:${s.runtimeID}:${s.taskID}`;
}

async function fetchJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  const data = await res.json();
  if (!res.ok) {
    throw new Error(data.error ?? `HTTP ${res.status}`);
  }
  return data as T;
}

export async function fetchTree(
  source: FileSource,
  path: string
): Promise<TreeEntry[]> {
  const src = encodeURIComponent(encodeSource(source));
  const p = encodeURIComponent(path || ".");
  return fetchJSON<TreeEntry[]>(`${BASE}/tree?source=${src}&path=${p}`);
}

export async function fetchFile(
  source: FileSource,
  path: string
): Promise<ReadResult> {
  const src = encodeURIComponent(encodeSource(source));
  const p = encodeURIComponent(path);
  return fetchJSON<ReadResult>(`${BASE}/read?source=${src}&path=${p}`);
}

export async function fetchDiff(
  source: FileSource,
  path: string
): Promise<DiffResult> {
  const src = encodeURIComponent(encodeSource(source));
  const p = encodeURIComponent(path);
  return fetchJSON<DiffResult>(`${BASE}/diff?source=${src}&path=${p}`);
}
