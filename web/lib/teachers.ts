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
export const CLIP_FILE = new RegExp(`^(\\d{3})_(${SPEEDS.join("|")})\\.wav$`);

export function hasTeacher(language: string): boolean {
  return language in CONFIG.teachers;
}

export function clipName(index: number, speed: string): string {
  return `${String(index).padStart(3, "0")}_${speed}.wav`;
}
