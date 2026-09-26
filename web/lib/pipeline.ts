import { execFile } from "node:child_process";
import { existsSync } from "node:fs";
import path from "node:path";
import { DATA_DIR } from "@/lib/songs";
import { REPO_ROOT } from "@/lib/teachers";

const YOUTUBE_ID = /(?:v=|youtu\.be\/|\/shorts\/|\/embed\/|\/live\/)([\w-]{11})/;
const STEP_TIMEOUT_MS = 300_000;

export class PipelineError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

/** Run one of the repo's Python scripts. Their stdout carries only counts and paths, never lyrics. */
export function runScript(script: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile("uv", ["run", script, ...args], { cwd: REPO_ROOT, timeout: STEP_TIMEOUT_MS }, (err, stdout, stderr) => {
      if (!err) return resolve(stdout);
      const reason = err.killed ? "執行逾時" : (stderr || err.message).split("\n").filter(Boolean).pop();
      reject(new PipelineError(502, `${script} 失敗：${reason}`));
    });
  });
}

export function parseVideoId(url: string): string | null {
  const trimmed = url.trim();
  if (/^[\w-]{11}$/.test(trimmed)) return trimmed;
  return trimmed.match(YOUTUBE_ID)?.[1] ?? null;
}

export type Stage = "transcribing" | "annotating" | "done" | "error";
export type JobStatus = { id: string; stage: Stage; message?: string; startedAt?: number };

const jobs = new Map<string, JobStatus>();

const transcriptPath = (id: string) => path.join(DATA_DIR, `${id}.json`);
const annotatedPath = (id: string) => path.join(DATA_DIR, `${id}.annotated.json`);

export function jobStatus(id: string): JobStatus | null {
  const job = jobs.get(id);
  if (job) return job;
  if (existsSync(annotatedPath(id))) return { id, stage: "done" };
  return null;
}

/**
 * Transcribe + annotate a YouTube video in the background. Resumes at annotation when a
 * transcript already exists (e.g. a previous run failed after transcribing).
 */
export function addSong(url: string): JobStatus {
  const id = parseVideoId(url);
  if (!id) throw new PipelineError(400, "看不懂這個網址，請貼上 YouTube 影片網址。");

  const existing = jobStatus(id);
  if (existing && existing.stage !== "error") return existing;

  const job: JobStatus = {
    id,
    stage: existsSync(transcriptPath(id)) ? "annotating" : "transcribing",
    startedAt: Date.now(),
  };
  jobs.set(id, job);

  (async () => {
    if (job.stage === "transcribing") {
      await runScript("transcribe.py", [`https://www.youtube.com/watch?v=${id}`, "--out", DATA_DIR]);
      job.stage = "annotating";
    }
    await runScript("annotate.py", [path.relative(REPO_ROOT, transcriptPath(id))]);
    job.stage = "done";
    // Files on disk are the source of truth once finished; only running/failed jobs live here.
    jobs.delete(id);
  })().catch((e) => {
    job.stage = "error";
    job.message = e instanceof PipelineError ? e.message : String(e);
  });

  return job;
}
