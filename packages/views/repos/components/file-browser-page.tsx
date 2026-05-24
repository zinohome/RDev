"use client";

import React, { useState, useCallback } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  ChevronRight,
  ChevronDown,
  File,
  Folder,
  FolderOpen,
  Loader2,
  AlertCircle,
  GitBranch,
} from "lucide-react";
import { api } from "@multica/core/api";
import { useWorkspaceId } from "@multica/core/hooks";
import type { RdevRepoTreeEntry } from "@multica/core/types";
import { cn } from "@multica/ui/lib/utils";
import { Button } from "@multica/ui/components/ui/button";

interface RepoRef {
  providerId: string;
  owner: string;
  repo: string;
  ref?: string;
}

function useRepoTree(repoRef: RepoRef | null, path?: string) {
  return useQuery({
    queryKey: ["rdev", "repos", repoRef?.providerId, repoRef?.owner, repoRef?.repo, repoRef?.ref, path],
    queryFn: () =>
      api.listRepoTree({
        providerId: repoRef!.providerId,
        owner: repoRef!.owner,
        repo: repoRef!.repo,
        ref: repoRef!.ref,
        path,
      }),
    enabled: !!repoRef,
  });
}

function useRepoFile(repoRef: RepoRef | null, path: string | null) {
  return useQuery({
    queryKey: ["rdev", "file", repoRef?.providerId, repoRef?.owner, repoRef?.repo, repoRef?.ref, path],
    queryFn: () =>
      api.getRepoFile({
        providerId: repoRef!.providerId,
        owner: repoRef!.owner,
        repo: repoRef!.repo,
        path: path!,
        ref: repoRef!.ref,
      }),
    enabled: !!repoRef && !!path,
  });
}

function TreeNode({
  entry,
  repoRef,
  depth,
  onFileSelect,
  selectedPath,
}: {
  entry: RdevRepoTreeEntry;
  repoRef: RepoRef;
  depth: number;
  onFileSelect: (path: string) => void;
  selectedPath: string | null;
}) {
  const [expanded, setExpanded] = useState(false);
  const isDir = entry.type === "tree";
  const isSelected = !isDir && selectedPath === entry.path;

  const subtree = useQuery({
    queryKey: ["rdev", "repos", repoRef.providerId, repoRef.owner, repoRef.repo, repoRef.ref, entry.path],
    queryFn: () =>
      api.listRepoTree({
        providerId: repoRef.providerId,
        owner: repoRef.owner,
        repo: repoRef.repo,
        ref: repoRef.ref,
        path: entry.path,
      }),
    enabled: isDir && expanded,
  });

  const toggle = useCallback(() => {
    if (isDir) setExpanded((v) => !v);
    else onFileSelect(entry.path);
  }, [isDir, entry.path, onFileSelect]);

  return (
    <div>
      <button
        type="button"
        onClick={toggle}
        className={cn(
          "flex w-full items-center gap-1.5 rounded px-2 py-1 text-sm text-left hover:bg-accent/60 transition-colors",
          isSelected && "bg-accent text-accent-foreground",
        )}
        style={{ paddingLeft: `${8 + depth * 16}px` }}
        aria-expanded={isDir ? expanded : undefined}
      >
        {isDir ? (
          <>
            {expanded ? (
              <ChevronDown className="size-3.5 shrink-0 text-muted-foreground" />
            ) : (
              <ChevronRight className="size-3.5 shrink-0 text-muted-foreground" />
            )}
            {expanded ? (
              <FolderOpen className="size-4 shrink-0 text-cyan-600" />
            ) : (
              <Folder className="size-4 shrink-0 text-cyan-600" />
            )}
          </>
        ) : (
          <>
            <span className="size-3.5 shrink-0" />
            <File className="size-4 shrink-0 text-muted-foreground" />
          </>
        )}
        <span className="truncate">{entry.name}</span>
      </button>

      {isDir && expanded && (
        <div>
          {subtree.isLoading && (
            <div style={{ paddingLeft: `${8 + (depth + 1) * 16}px` }} className="flex items-center gap-1.5 py-1 text-xs text-muted-foreground">
              <Loader2 className="size-3 animate-spin" />
              Loading…
            </div>
          )}
          {subtree.data?.map((child) => (
            <TreeNode
              key={child.path}
              entry={child}
              repoRef={repoRef}
              depth={depth + 1}
              onFileSelect={onFileSelect}
              selectedPath={selectedPath}
            />
          ))}
        </div>
      )}
    </div>
  );
}

function CodeViewer({ content, path }: { content: string; path: string }) {
  const ext = path.split(".").pop()?.toLowerCase() ?? "";
  const langMap: Record<string, string> = {
    ts: "typescript", tsx: "tsx", js: "javascript", jsx: "jsx",
    py: "python", go: "go", rs: "rust", yaml: "yaml", yml: "yaml",
    json: "json", md: "markdown", sh: "bash", toml: "toml",
  };
  const lang = langMap[ext] ?? "text";

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 border-b border-border px-4 py-2 text-xs text-muted-foreground bg-muted/30">
        <File className="size-3.5" />
        <span className="font-mono">{path}</span>
        <span className="ml-auto">{lang}</span>
      </div>
      <div className="flex-1 overflow-auto">
        <pre className="p-4 text-xs font-mono leading-relaxed whitespace-pre text-foreground min-w-max">
          <code>{content}</code>
        </pre>
      </div>
    </div>
  );
}

interface FileBrowserPageProps {
  initialProviderId?: string;
  initialOwner?: string;
  initialRepo?: string;
}

export function FileBrowserPage({
  initialProviderId,
  initialOwner,
  initialRepo,
}: FileBrowserPageProps) {
  const [selectedFile, setSelectedFile] = useState<string | null>(null);

  const repoRef: RepoRef | null =
    initialProviderId && initialOwner && initialRepo
      ? { providerId: initialProviderId, owner: initialOwner, repo: initialRepo }
      : null;

  const rootTree = useRepoTree(repoRef);
  const fileContent = useRepoFile(repoRef, selectedFile);

  if (!repoRef) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-4 text-muted-foreground">
        <GitBranch className="size-12 text-cyan-600" />
        <div className="text-center">
          <p className="text-sm font-medium">Select a repository</p>
          <p className="text-xs mt-1">Navigate to a repository to browse its files.</p>
        </div>
      </div>
    );
  }

  return (
    <div className="flex h-full">
      {/* Left: file tree */}
      <div className="w-64 shrink-0 border-r border-border flex flex-col overflow-hidden">
        <div className="flex items-center gap-2 border-b border-border px-3 py-2.5">
          <GitBranch className="size-4 text-cyan-600 shrink-0" />
          <span className="text-sm font-medium truncate">{initialOwner}/{initialRepo}</span>
        </div>
        <div className="flex-1 overflow-y-auto py-1">
          {rootTree.isLoading && (
            <div className="flex items-center gap-2 px-4 py-6 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin" />
              Loading tree…
            </div>
          )}
          {rootTree.isError && (
            <div className="flex items-center gap-2 px-4 py-4 text-sm text-destructive">
              <AlertCircle className="size-4 shrink-0" />
              Failed to load repository
            </div>
          )}
          {rootTree.data?.map((entry) => (
            <TreeNode
              key={entry.path}
              entry={entry}
              repoRef={repoRef}
              depth={0}
              onFileSelect={setSelectedFile}
              selectedPath={selectedFile}
            />
          ))}
        </div>
      </div>

      {/* Right: code viewer */}
      <div className="flex-1 overflow-hidden">
        {!selectedFile && (
          <div className="flex flex-col items-center justify-center h-full text-muted-foreground gap-2">
            <File className="size-10" />
            <p className="text-sm">Select a file to view its contents</p>
          </div>
        )}
        {selectedFile && fileContent.isLoading && (
          <div className="flex items-center justify-center h-full gap-2 text-muted-foreground">
            <Loader2 className="size-5 animate-spin" />
            <span>Loading file…</span>
          </div>
        )}
        {selectedFile && fileContent.isError && (
          <div className="flex items-center justify-center h-full gap-2 text-destructive">
            <AlertCircle className="size-5" />
            <span>Failed to load file</span>
          </div>
        )}
        {selectedFile && fileContent.data && (
          <CodeViewer
            content={
              fileContent.data.encoding === "base64"
                ? atob(fileContent.data.content)
                : fileContent.data.content
            }
            path={selectedFile}
          />
        )}
      </div>
    </div>
  );
}

export function ReposPage() {
  return (
    <div className="flex flex-col h-full">
      <div className="border-b border-border px-6 py-4">
        <h1 className="text-lg font-semibold">Repositories</h1>
        <p className="text-sm text-muted-foreground mt-0.5">
          Browse connected code repositories
        </p>
      </div>
      <div className="flex-1 overflow-hidden">
        <FileBrowserPage />
      </div>
    </div>
  );
}
