import path from "node:path";
import { PipelineError, runScript } from "@/lib/pipeline";
import { DATA_DIR } from "@/lib/songs";
import { REPO_ROOT } from "@/lib/teachers";

export class ReviewError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

const running = new Set<string>();

/** Re-run annotate.py on the (edited) transcript: one Gemini Flash request, no TTS quota. */
export async function reannotate(videoId: string): Promise<string> {
  if (running.has(videoId)) throw new ReviewError(409, "這首歌正在重新分析中。");
  running.add(videoId);
  try {
    const stdout = await runScript("annotate.py", [path.relative(REPO_ROOT, path.join(DATA_DIR, `${videoId}.json`))]);
    return stdout.split("\n").filter((l) => /finish_reason|annotated:|review|cover/.test(l)).join("\n");
  } catch (e) {
    throw e instanceof PipelineError ? new ReviewError(e.status, `重新分析失敗：${e.message}`) : e;
  } finally {
    running.delete(videoId);
  }
}
