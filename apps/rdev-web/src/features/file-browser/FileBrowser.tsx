"use client";

import { useState } from "react";
import { FileTree } from "./FileTree";
import { FileViewer } from "./FileViewer";
import type { FileSource, VCSSource, RuntimeSource } from "./api";

type SourceTab = "vcs" | "runtime";

interface FileBrowserProps {
  // VCS source config
  vcsProviderID?: string;
  vcsOwner?: string;
  vcsRepo?: string;
  vcsBranch?: string;
  // Runtime source config
  runtimeID?: string;
  taskID?: string;
}

export function FileBrowser({
  vcsProviderID = "gitea",
  vcsOwner,
  vcsRepo,
  vcsBranch = "main",
  runtimeID,
  taskID,
}: FileBrowserProps) {
  const hasVCS = Boolean(vcsOwner && vcsRepo);
  const hasRuntime = Boolean(runtimeID && taskID);

  const [activeTab, setActiveTab] = useState<SourceTab>(
    hasVCS ? "vcs" : "runtime"
  );
  const [selectedFile, setSelectedFile] = useState<string | null>(null);

  const vcsSource: VCSSource = {
    kind: "vcs",
    providerID: vcsProviderID,
    owner: vcsOwner ?? "",
    repo: vcsRepo ?? "",
    branch: vcsBranch,
  };

  const runtimeSource: RuntimeSource = {
    kind: "runtime",
    runtimeID: runtimeID ?? "",
    taskID: taskID ?? "",
  };

  const source: FileSource = activeTab === "vcs" ? vcsSource : runtimeSource;

  if (!hasVCS && !hasRuntime) {
    return (
      <div className="flex items-center justify-center h-full text-sm text-gray-400">
        No file sources configured
      </div>
    );
  }

  return (
    <div className="flex flex-col h-full border border-gray-200 rounded-lg overflow-hidden bg-white">
      {/* Source tab bar */}
      <div className="flex items-center gap-0 border-b border-gray-200 bg-gray-50 px-3 pt-2">
        {hasVCS && (
          <button
            onClick={() => {
              setActiveTab("vcs");
              setSelectedFile(null);
            }}
            className={[
              "px-4 py-2 text-sm rounded-t border-x border-t -mb-px",
              activeTab === "vcs"
                ? "bg-white border-gray-200 text-gray-800 font-medium"
                : "border-transparent text-gray-500 hover:text-gray-700",
            ].join(" ")}
          >
            Gitea / GitHub
          </button>
        )}
        {hasRuntime && (
          <button
            onClick={() => {
              setActiveTab("runtime");
              setSelectedFile(null);
            }}
            className={[
              "px-4 py-2 text-sm rounded-t border-x border-t -mb-px",
              activeTab === "runtime"
                ? "bg-white border-gray-200 text-gray-800 font-medium"
                : "border-transparent text-gray-500 hover:text-gray-700",
            ].join(" ")}
          >
            Live Workspace
          </button>
        )}
      </div>

      {/* Main layout: file tree + viewer */}
      <div className="flex flex-1 min-h-0">
        {/* Left: file tree */}
        <div className="w-56 flex-shrink-0 border-r border-gray-200 overflow-auto">
          <FileTree
            source={source}
            onFileSelect={setSelectedFile}
            selectedPath={selectedFile ?? undefined}
          />
        </div>

        {/* Right: file viewer */}
        <div className="flex-1 min-w-0">
          <FileViewer source={source} path={selectedFile} />
        </div>
      </div>
    </div>
  );
}
