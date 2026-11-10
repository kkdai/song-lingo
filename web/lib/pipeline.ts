import { execFile } from "node:child_process";
import { REPO_ROOT } from "@/lib/teachers";

const STEP_TIMEOUT_MS = 300_000;

export class PipelineError extends Error {
  constructor(
    public status: number,
    message: string,
    /** The script's own last stderr line, without the "<script> 失敗：" prefix. */
    public reason = message,
  ) {
    super(message);
  }
}

/** Run one of the repo's Python scripts. Their stdout carries only counts and paths, never lyrics. */
export function runScript(script: string, args: string[]): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile("uv", ["run", script, ...args], { cwd: REPO_ROOT, timeout: STEP_TIMEOUT_MS }, (err, stdout, stderr) => {
      if (!err) return resolve(stdout);
      const reason = err.killed ? "執行逾時" : ((stderr || err.message).split("\n").filter(Boolean).pop() ?? "");
      reject(new PipelineError(502, `${script} 失敗：${reason}`, reason));
    });
  });
}
