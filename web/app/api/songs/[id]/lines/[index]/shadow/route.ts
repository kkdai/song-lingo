import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import type { NextRequest } from "next/server";
import { PipelineError, runScript } from "@/lib/pipeline";
import { addAttempt, updateProgress } from "@/lib/progress";
import { isVideoId, readSong } from "@/lib/songs";

const MAX_BYTES = 5 * 1024 * 1024;
// Browser recording formats → a MIME type gemini-3.5-transcribe accepts, and a file extension.
const FORMATS: Record<string, [mime: string, ext: string]> = {
  "audio/webm": ["audio/webm", "webm"],
  "audio/ogg": ["audio/ogg", "ogg"],
  "audio/mp4": ["audio/m4a", "m4a"], // Safari / iOS
  "audio/m4a": ["audio/m4a", "m4a"],
  "audio/x-m4a": ["audio/m4a", "m4a"],
  "audio/aac": ["audio/aac", "aac"],
  "audio/wav": ["audio/wav", "wav"],
};

// One scoring at a time, and no calls at all while the transcribe quota is exhausted:
// a double-tap or a retry storm must not be billed twice.
let busy = false;
let quotaUntil = 0;

function fail(status: number, error: string) {
  return Response.json({ error }, { status });
}

/**
 * Score a shadowing attempt: the learner's recording of one line is transcribed with
 * gemini-3.5-transcribe (via shadow.py) and compared with that line. The recording is written
 * to a temp dir only for the call and deleted right after; nothing is logged or stored except
 * the score and missed words, which go into the song's study progress.
 */
export async function POST(request: NextRequest, ctx: RouteContext<"/api/songs/[id]/lines/[index]/shadow">) {
  const { id, index } = await ctx.params;
  if (!isVideoId(id) || !/^\d+$/.test(index)) return fail(404, "Not found");

  if (Date.now() < quotaUntil) {
    const minutes = Math.ceil((quotaUntil - Date.now()) / 60_000);
    return fail(429, `跟讀評分的額度已用完，大約 ${minutes} 分鐘後再試。`);
  }
  if (busy) return fail(429, "上一次跟讀還在評分中，請稍等。");

  const song = await readSong(id);
  const line = song?.lines[Number(index)];
  if (!song || !line) return fail(404, "找不到這一句。");

  const form = await request.formData().catch(() => null);
  const audio = form?.get("audio");
  if (!(audio instanceof Blob)) return fail(400, "沒有收到錄音。");
  if (audio.size < 1000) return fail(422, "錄音太短了，請按住按鈕念完整句再放開。");
  if (audio.size > MAX_BYTES) return fail(413, "錄音太長了，請只念這一句。");
  const format = FORMATS[audio.type.split(";")[0].trim().toLowerCase()];
  if (!format) return fail(415, `不支援這種錄音格式（${audio.type || "未知"}）。`);

  busy = true;
  const dir = await mkdtemp(path.join(os.tmpdir(), "song-lingo-shadow-"));
  try {
    const audioPath = path.join(dir, `recording.${format[1]}`);
    const expectedPath = path.join(dir, "expected.json");
    await writeFile(audioPath, Buffer.from(await audio.arrayBuffer()));
    await writeFile(
      expectedPath,
      JSON.stringify({
        language: song.language,
        text: line.text,
        reading: line.reading ?? "",
        tokens: (line.tokens ?? []).map((t) => ({ surface: t.surface, reading: t.reading })),
      }),
    );
    const stdout = await runScript("shadow.py", [audioPath, format[0], expectedPath]);
    const result: { score: number; words: { surface: string; status: string }[] } = JSON.parse(stdout.trim().split("\n").pop()!);
    const attempt = {
      at: new Date().toISOString(),
      score: result.score,
      missed: result.words.filter((w) => w.status !== "ok").map((w) => w.surface),
    };
    // The transcription is already paid for: if saving fails, still return the score (marked
    // unsaved) rather than an error that would make the learner record and pay again.
    let saved = true;
    try {
      await updateProgress(id, (p) => {
        p.lastLine = Number(index);
        addAttempt(p, line.text, attempt);
      });
    } catch (e) {
      saved = false;
      console.error(`[shadow] progress not saved for ${id}: ${e instanceof Error ? e.message : e}`);
    }
    return Response.json({ ...result, attempt, saved });
  } catch (e) {
    if (!(e instanceof PipelineError)) throw e;
    const quota = e.reason.match(/^QUOTA (\d+)/);
    if (quota) {
      quotaUntil = Date.now() + Math.max(Number(quota[1]), 60) * 1000;
      return fail(429, "跟讀評分的額度已用完，請稍後再試。");
    }
    if (e.reason.startsWith("沒有聽到聲音")) return fail(422, e.reason);
    return fail(502, `評分失敗：${e.reason}`);
  } finally {
    busy = false;
    await rm(dir, { recursive: true, force: true });
  }
}
