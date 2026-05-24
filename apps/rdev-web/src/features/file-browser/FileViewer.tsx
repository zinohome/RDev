"use client";

import { useEffect, useState } from "react";
import { fetchFile, fetchDiff, type FileSource } from "./api";

type ViewMode = "content" | "diff";

interface FileViewerProps {
  source: FileSource;
  path: string | null;
}

export function FileViewer({ source, path }: FileViewerProps) {
  const [mode, setMode] = useState<ViewMode>("content");
  const [content, setContent] = useState<string | null>(null);
  const [encoding, setEncoding] = useState<string>("utf-8");
  const [truncated, setTruncated] = useState(false);
  const [diff, setDiff] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!path) return;
    setContent(null);
    setDiff(null);
    setError(null);
    setLoading(true);

    const load = async () => {
      try {
        if (mode === "content") {
          const r = await fetchFile(source, path);
          setContent(r.content ?? "");
          setEncoding(r.encoding);
          setTruncated(r.truncated);
        } else {
          const r = await fetchDiff(source, path);
          setDiff(r.patch);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load");
      } finally {
        setLoading(false);
      }
    };
    load();
  }, [path, mode, source]);

  if (!path) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-gray-400">
        Select a file to view
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full">
      <div className="flex items-center gap-2 px-3 py-2 border-b border-gray-200 bg-gray-50">
        <span className="text-sm font-mono text-gray-600 flex-1 truncate">
          {path}
        </span>
        <div className="flex gap-1">
          {(["content", "diff"] as ViewMode[]).map((m) => (
            <button
              key={m}
              onClick={() => setMode(m)}
              className={[
                "px-3 py-1 text-xs rounded border",
                mode === m
                  ? "bg-blue-500 text-white border-blue-500"
                  : "bg-white text-gray-600 border-gray-300 hover:bg-gray-50",
              ].join(" ")}
            >
              {m === "content" ? "View" : "Diff"}
            </button>
          ))}
        </div>
      </div>

      <div className="flex-1 overflow-auto bg-white">
        {loading && (
          <div className="p-4 text-sm text-gray-400">Loading…</div>
        )}
        {error && (
          <div className="p-4 text-sm text-red-500">Error: {error}</div>
        )}
        {!loading && !error && mode === "content" && (
          <>
            {encoding === "binary" ? (
              <div className="p-4 text-sm text-gray-400">
                Binary file — cannot display
              </div>
            ) : (
              <>
                {truncated && (
                  <div className="px-3 py-1 text-xs text-amber-600 bg-amber-50 border-b border-amber-200">
                    File truncated at 5 MB
                  </div>
                )}
                <pre className="p-4 text-xs font-mono text-gray-800 whitespace-pre-wrap break-words">
                  {content}
                </pre>
              </>
            )}
          </>
        )}
        {!loading && !error && mode === "diff" && (
          <pre className="p-4 text-xs font-mono whitespace-pre-wrap break-words">
            {diff ? (
              diff.split("\n").map((line, i) => (
                <span
                  key={i}
                  className={
                    line.startsWith("+")
                      ? "text-green-700 bg-green-50 block"
                      : line.startsWith("-")
                        ? "text-red-700 bg-red-50 block"
                        : line.startsWith("@@")
                          ? "text-blue-600 block"
                          : "text-gray-700 block"
                  }
                >
                  {line}
                </span>
              ))
            ) : (
              <span className="text-gray-400">No diff available</span>
            )}
          </pre>
        )}
      </div>
    </div>
  );
}
