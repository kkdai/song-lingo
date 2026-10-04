// Progress types and rules shared by the server (lib/progress.ts) and the study page.
// Kept free of Node imports so the client bundle can use it.

/** Attempts kept per line; older ones are dropped. */
export const MAX_ATTEMPTS = 20;

export type ShadowAttempt = {
  at: string;
  score: number;
  /** Words that weren't heard as written (for "words I keep missing"). */
  missed: string[];
};

export type LineProgress = {
  learned?: boolean;
  shadow?: ShadowAttempt[];
};

/** The line's progress with one more shadowing attempt, keeping only the latest MAX_ATTEMPTS. */
export function withAttempt(line: LineProgress | undefined, attempt: ShadowAttempt): LineProgress {
  return { ...line, shadow: [...(line?.shadow ?? []), attempt].slice(-MAX_ATTEMPTS) };
}
