// Song types as the Go API (backend/internal/songs) returns them.
import type { ProgressSummary } from "@/lib/progress";

const VIDEO_ID = /^[\w-]{11}$/;

export type Token = {
  surface: string;
  reading: string;
  pos: string;
  meaning_zh: string;
};

export type Line = {
  start: string;
  end: string;
  text: string;
  reading?: string | null;
  section?: string | null;
  uncertain: boolean;
  translation_zh?: string;
  tokens?: Token[];
  romanization?: string;
  grammar_note?: string;
  pronunciation_tip?: string;
  needs_review?: boolean;
  /** Set by the review UI. */
  reviewed?: boolean;
  translation_zh_manual?: string;
  /** Text or reading was edited; tokens/romanization are outdated until re-annotated. */
  stale?: boolean;
  /** Teacher clip URLs per speed; generated on first request if not cached yet. */
  audio?: Record<string, string>;
  /** Speeds whose clip already exists on disk (plays instantly, no TTS request). */
  audioCached?: string[];
  /** This line's key in the song's progress (a hash of its text). */
  progressKey?: string;
};

export type Song = {
  id: string;
  language: string;
  title_guess: string | null;
  artist_guess: string | null;
  source: string;
  lines: Line[];
};

export type SongSummary = Pick<Song, "id" | "language" | "title_guess" | "artist_guess"> & {
  lineCount: number;
  hasTeacher: boolean;
  progress: ProgressSummary;
};

export function isVideoId(id: string): boolean {
  return VIDEO_ID.test(id);
}
