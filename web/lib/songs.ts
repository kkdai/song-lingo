import { readdir, readFile } from "node:fs/promises";
import path from "node:path";
import { SPEEDS, clipName, hasTeacher } from "@/lib/teachers";

export const DATA_DIR = process.env.SONG_DATA_DIR ?? path.join(process.cwd(), "..", "output");

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
  /** Teacher clip URLs per speed; generated on first request if not cached yet. */
  audio?: Record<string, string>;
  /** Speeds whose clip already exists on disk (plays instantly, no TTS request). */
  audioCached?: string[];
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
};

export function isVideoId(id: string): boolean {
  return VIDEO_ID.test(id);
}

export async function readSong(id: string): Promise<Song | null> {
  try {
    const raw = await readFile(path.join(DATA_DIR, `${id}.annotated.json`), "utf8");
    return { id, ...JSON.parse(raw) };
  } catch {
    return null;
  }
}

export async function listSongs(): Promise<SongSummary[]> {
  let files: string[];
  try {
    files = await readdir(DATA_DIR);
  } catch {
    return [];
  }
  const ids = files.filter((f) => f.endsWith(".annotated.json")).map((f) => f.split(".")[0]);
  const songs = await Promise.all(ids.filter(isVideoId).map(readSong));
  return songs
    .filter((s): s is Song => s !== null)
    .map(({ id, language, title_guess, artist_guess, lines }) => ({
      id,
      language,
      title_guess,
      artist_guess,
      lineCount: lines.length,
      hasTeacher: hasTeacher(language),
    }));
}

/** Unique line texts in first-seen order; a line's clip index is its position here (as in speak.py). */
export function uniqueTexts(song: Song): string[] {
  return [...new Set(song.lines.map((l) => l.text))];
}

export async function getSong(id: string): Promise<Song | null> {
  if (!isVideoId(id)) return null;
  const song = await readSong(id);
  if (!song) return null;
  if (!hasTeacher(song.language)) {
    for (const line of song.lines) delete line.audio;
    return song;
  }

  const cached = new Set(await readdir(path.join(DATA_DIR, "audio", id)).catch(() => []));
  const index = new Map(uniqueTexts(song).map((text, i) => [text, i]));
  for (const line of song.lines) {
    const i = index.get(line.text)!;
    line.audio = Object.fromEntries(SPEEDS.map((speed) => [speed, `/api/audio/${id}/${clipName(i, speed)}`]));
    line.audioCached = SPEEDS.filter((speed) => cached.has(clipName(i, speed)));
  }
  return song;
}
