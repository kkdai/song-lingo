// `npm run dev`: start the Go API (backend/) and the Next.js dev server together.
// Stopping either one (or Ctrl-C) stops both.
import { execFileSync, spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const web = path.join(path.dirname(fileURLToPath(import.meta.url)), "..");
const backend = path.join(web, "..", "backend");
// Build, then run the binary itself: `go run` doesn't pass SIGTERM on to it.
execFileSync("go", ["build", "-o", "bin/song-lingo", "./cmd/song-lingo"], { cwd: backend, stdio: "inherit" });
const children = [
  spawn(path.join(backend, "bin", "song-lingo"), ["serve"], { cwd: backend, stdio: "inherit" }),
  spawn(path.join(web, "node_modules", ".bin", "next"), ["dev", ...process.argv.slice(2)], { cwd: web, stdio: "inherit" }),
];

let exiting = false;
function stopAll(code) {
  if (exiting) return;
  exiting = true;
  for (const child of children) if (child.exitCode === null) child.kill("SIGTERM");
  process.exitCode = code;
}
for (const child of children) child.on("exit", (code) => stopAll(code ?? 1));
for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => stopAll(0));
