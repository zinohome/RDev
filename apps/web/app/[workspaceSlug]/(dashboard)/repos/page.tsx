"use client";

import { ReposPage } from "@multica/views/repos";
import { ErrorBoundary } from "@multica/ui/components/common/error-boundary";

export default function Page() {
  return (
    <ErrorBoundary>
      <ReposPage />
    </ErrorBoundary>
  );
}
