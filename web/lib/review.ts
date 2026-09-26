import { execFile } from "node:child_process";
import { readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { DATA_DIR, type Line, type Song } from "@/lib/songs";
import { REPO_ROOT } from "@/lib/teachers";

const MAX_FIELD_LENGTH = 500;

export type LineEdit = {
  text?: string;
  reading?: string;
  translation_zh?: string;
  reviewed?: boolean;
  /** Apply to every line with the same original text (repeated choruses). */
  applyToSame?: boolean;
};

export class ReviewError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

type SongFile = Omit<Song, "id">;

async function readJson(file: string): Promise<SongFile> {
  try {
    return JSON.parse(await readFile(file, "utf8"));
  } catch {
    throw new ReviewError(404, "找不到歌曲檔案。");
  }
}

async function writeJson(file: string, data: SongFile) {
  await writeFile(`${file}.tmp`, JSON.stringify(data, null, 2));
  await rename(`${file}.tmp`, file);
}

function cleanString(value: unknown, field: string): string | undefined {
  if (value === undefined) return undefined;
  if (typeof value !== "string" || value.length > MAX_FIELD_LENGTH) {
    throw new ReviewError(400, `${field} 格式不正確。`);
  }
  return value.trim();
}

export function parseEdit(body: unknown): LineEdit {
  if (!body || typeof body !== "object") throw new ReviewError(400, "請求格式不正確。");
  const b = body as Record<string, unknown>;
  const text = cleanString(b.text, "歌詞");
  if (text === "") throw new ReviewError(400, "歌詞不能是空的。");
  if (b.reviewed !== undefined && typeof b.reviewed !== "boolean") throw new ReviewError(400, "reviewed 格式不正確。");
  return {
    text,
    reading: cleanString(b.reading, "讀音"),
    translation_zh: cleanString(b.translation_zh, "翻譯"),
    reviewed: b.reviewed as boolean | undefined,
    applyToSame: b.applyToSame === true,
  };
}

/**
 * Apply a review edit to both the transcript (source of truth for annotate.py) and the
 * annotated file the web app reads. Text/reading changes mark the annotation stale until
 * the song is re-annotated; manual translations and the reviewed flag survive re-annotation.
 */
export async function editLine(videoId: string, index: number, edit: LineEdit): Promise<number> {
  const transcriptFile = path.join(DATA_DIR, `${videoId}.json`);
  const annotatedFile = path.join(DATA_DIR, `${videoId}.annotated.json`);
  const [transcript, annotated] = await Promise.all([readJson(transcriptFile), readJson(annotatedFile)]);
  if (transcript.lines.length !== annotated.lines.length) {
    throw new ReviewError(409, "轉錄檔和標註檔的句數不一致，請重新執行 annotate.py。");
  }
  if (!Number.isInteger(index) || index < 0 || index >= annotated.lines.length) {
    throw new ReviewError(404, "找不到這一句。");
  }

  const original = annotated.lines[index].text;
  const targets = edit.applyToSame
    ? annotated.lines.flatMap((l, i) => (l.text === original ? [i] : []))
    : [index];

  for (const i of targets) {
    const t: Line = transcript.lines[i];
    const a: Line = annotated.lines[i];
    if (edit.text !== undefined && edit.text !== a.text) {
      t.text = a.text = edit.text;
      a.stale = true;
    }
    if (edit.reading !== undefined && edit.reading !== (a.reading ?? "")) {
      t.reading = a.reading = edit.reading || null;
      a.stale = true;
    }
    if (edit.translation_zh !== undefined && edit.translation_zh !== (a.translation_zh ?? "")) {
      if (edit.translation_zh) {
        t.translation_zh_manual = a.translation_zh_manual = a.translation_zh = edit.translation_zh;
      } else {
        delete t.translation_zh_manual;
        delete a.translation_zh_manual;
      }
    }
    if (edit.reviewed !== undefined) {
      t.reviewed = a.reviewed = edit.reviewed;
      if (edit.reviewed) {
        t.uncertain = a.uncertain = false;
        a.needs_review = false;
      }
    }
  }

  await writeJson(transcriptFile, transcript);
  await writeJson(annotatedFile, annotated);
  return targets.length;
}

const running = new Set<string>();

/** Re-run annotate.py on the (edited) transcript: one Gemini Flash request, no TTS quota. */
export async function reannotate(videoId: string): Promise<string> {
  if (running.has(videoId)) throw new ReviewError(409, "這首歌正在重新分析中。");
  running.add(videoId);
  try {
    const transcript = path.relative(REPO_ROOT, path.join(DATA_DIR, `${videoId}.json`));
    return await new Promise((resolve, reject) => {
      execFile("uv", ["run", "annotate.py", transcript], { cwd: REPO_ROOT, timeout: 180_000 }, (err, stdout, stderr) => {
        // annotate.py prints only counts and file paths, never lyrics.
        const summary = stdout.split("\n").filter((l) => /finish_reason|annotated:|review|cover/.test(l)).join("\n");
        if (err) reject(new ReviewError(502, `重新分析失敗：${(stderr || err.message).split("\n").filter(Boolean).pop()}`));
        else resolve(summary);
      });
    });
  } finally {
    running.delete(videoId);
  }
}
