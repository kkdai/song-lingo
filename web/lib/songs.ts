import { readdir, readFile } from "node:fs/promises";
import path from "node:path";

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
  audio?: { normal: string; slow: string };
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
  hasAudio: boolean;
};

export function isVideoId(id: string): boolean {
  return VIDEO_ID.test(id);
}

async function readSong(id: string): Promise<Song | null> {
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
      hasAudio: lines.some((l) => l.audio),
    }));
}

export async function getSong(id: string): Promise<Song | null> {
  if (!isVideoId(id)) return null;
  const song = await readSong(id);
  if (!song) return null;
  // speak.py stores local file paths; the browser fetches them through the audio route.
  for (const line of song.lines) {
    if (line.audio) {
      line.audio = {
        normal: `/api/audio/${id}/${path.basename(line.audio.normal)}`,
        slow: `/api/audio/${id}/${path.basename(line.audio.slow)}`,
      };
    }
  }
  return song;
}
