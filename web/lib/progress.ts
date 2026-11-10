// Study progress as the Go API (backend/internal/progress) returns it.
import type { LineProgress } from "@/lib/progressShared";

export type { LineProgress, ShadowAttempt } from "@/lib/progressShared";

/**
 * What the learner has done in one song. Lines are keyed by a hash of their text (`progressKey`):
 * repeated choruses share one entry, and no lyrics are copied into the progress store.
 */
export type Progress = {
  version: 1;
  lastLine: number;
  lastStudiedAt: string | null;
  lines: Record<string, LineProgress>;
};

export type ProgressSummary = {
  learned: number;
  total: number;
  lastLine: number;
  lastStudiedAt: string | null;
};
