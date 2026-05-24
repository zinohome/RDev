"use client";

import { useState } from "react";
import { fetchTree, type FileSource, type TreeEntry } from "./api";

interface FileTreeProps {
  source: FileSource;
  rootPath?: string;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
}

interface TreeNodeProps {
  entry: TreeEntry;
  source: FileSource;
  depth: number;
  onFileSelect: (path: string) => void;
  selectedPath?: string;
}

function TreeNode({
  entry,
  source,
  depth,
  onFileSelect,
  selectedPath,
}: TreeNodeProps) {
  const [children, setChildren] = useState<TreeEntry[] | null>(null);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const indent = depth * 16;
  const isSelected = selectedPath === entry.path;

  async function toggleDir() {
    if (!entry.is_dir) return;
    if (!open && children === null) {
      setLoading(true);
      setError(null);
      try {
        const entries = await fetchTree(source, entry.path);
        setChildren(entries);
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load");
      } finally {
        setLoading(false);
      }
    }
    setOpen((v) => !v);
  }

  return (
    <div>
      <div
        style={{ paddingLeft: indent }}
        className={[
          "flex items-center gap-1 px-2 py-0.5 cursor-pointer rounded text-sm",
          isSelected
            ? "bg-blue-100 text-blue-800"
            : "hover:bg-gray-100 text-gray-700",
        ].join(" ")}
        onClick={() => {
          if (entry.is_dir) {
            toggleDir();
          } else {
            onFileSelect(entry.path);
          }
        }}
      >
        <span className="select-none">
          {entry.is_dir ? (open ? "▾" : "▸") : "·"}
        </span>
        <span className={entry.is_dir ? "font-medium" : ""}>{entry.name}</span>
        {loading && (
          <span className="text-xs text-gray-400 ml-1">loading…</span>
        )}
      </div>
      {error && (
        <div
          style={{ paddingLeft: indent + 20 }}
          className="text-xs text-red-500 px-2"
        >
          {error}
        </div>
      )}
      {open && children && (
        <div>
          {children.map((child) => (
            <TreeNode
              key={child.path}
              entry={child}
              source={source}
              depth={depth + 1}
              onFileSelect={onFileSelect}
              selectedPath={selectedPath}
            />
          ))}
          {children.length === 0 && (
            <div
              style={{ paddingLeft: indent + 20 }}
              className="text-xs text-gray-400 px-2 py-0.5"
            >
              (empty)
            </div>
          )}
        </div>
      )}
    </div>
  );
}

export function FileTree({
  source,
  rootPath = ".",
  onFileSelect,
  selectedPath,
}: FileTreeProps) {
  const [entries, setEntries] = useState<TreeEntry[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Load root on mount
  if (entries === null && !loading && !error) {
    setLoading(true);
    fetchTree(source, rootPath)
      .then((e) => {
        setEntries(e);
        setLoading(false);
      })
      .catch((e) => {
        setError(e instanceof Error ? e.message : "Failed to load tree");
        setLoading(false);
      });
  }

  if (loading) {
    return (
      <div className="p-4 text-sm text-gray-400">Loading file tree…</div>
    );
  }
  if (error) {
    return <div className="p-4 text-sm text-red-500">Error: {error}</div>;
  }
  if (!entries) return null;

  return (
    <div className="overflow-auto h-full py-1">
      {entries.map((entry) => (
        <TreeNode
          key={entry.path}
          entry={entry}
          source={source}
          depth={0}
          onFileSelect={onFileSelect}
          selectedPath={selectedPath}
        />
      ))}
      {entries.length === 0 && (
        <div className="p-4 text-sm text-gray-400">(empty repository)</div>
      )}
    </div>
  );
}
