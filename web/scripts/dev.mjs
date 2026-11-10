// `npm run dev`: build and start the Go API (backend/), then the Next.js dev server.
import { execFileSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runTogether } from "./together.mjs";

const web = path.join(path.dirname(fileURLToPath(import.meta.url)), "..");
const backend = path.join(web, "..", "backend");
// Build, then run the binary itself: `go run` doesn't pass SIGTERM on to it.
execFileSync("go", ["build", "-o", "bin/song-lingo", "./cmd/song-lingo"], { cwd: backend, stdio: "inherit" });
runTogether([
  { command: path.join(backend, "bin", "song-lingo"), args: ["serve"], cwd: backend },
  { command: path.join(web, "node_modules", ".bin", "next"), args: ["dev", ...process.argv.slice(2)], cwd: web },
]);
