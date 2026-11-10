// Production (the Docker image): the Go API on localhost and `next start` on $PORT.
// The song-lingo binary is taken from $PATH.
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runTogether } from "./together.mjs";

const web = path.join(path.dirname(fileURLToPath(import.meta.url)), "..");
runTogether([
  { command: "song-lingo", args: ["serve"], cwd: web },
  { command: path.join(web, "node_modules", ".bin", "next"), args: ["start"], cwd: web },
]);
