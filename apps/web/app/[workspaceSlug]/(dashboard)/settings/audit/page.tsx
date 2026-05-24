"use client";

import { AuditLogPage } from "@multica/views/settings";
import { ErrorBoundary } from "@multica/ui/components/common/error-boundary";

export default function Page() {
  return (
    <ErrorBoundary>
      <AuditLogPage />
    </ErrorBoundary>
  );
}
