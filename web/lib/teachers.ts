import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import path from "node:path";

export const REPO_ROOT = path.join(process.cwd(), "..");

type TeacherConfig = {
  model: string;
  teachers: Record<string, { language_code: string; display_name: string; description: string }>;
  styles: Record<string, string>;
};

// Shared with speak.py.
export const CONFIG: TeacherConfig = JSON.parse(
  readFileSync(path.join(REPO_ROOT, "config", "teachers.json"), "utf8"),
);

export const SPEEDS = Object.keys(CONFIG.styles);
export const CLIP_FILE = new RegExp(`^([0-9a-f]{16})_(${SPEEDS.join("|")})\\.wav$`);

export function hasTeacher(language: string): boolean {
  return language in CONFIG.teachers;
}

/** Clips are keyed by line content (same as speak.py), so editing a lyric only invalidates that line. */
export function textHash(text: string): string {
  return createHash("sha1").update(text).digest("hex").slice(0, 16);
}

export function clipName(text: string, speed: string): string {
  return `${textHash(text)}_${speed}.wav`;
}
