// Production (the Docker image): the Go API on localhost and the standalone Next.js server on $PORT.
// The song-lingo binary is taken from $PATH.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { runTogether } from "./together.mjs";

const web = path.join(path.dirname(fileURLToPath(import.meta.url)), "..");
const server = fs.existsSync(path.join(web, "server.js"))
  ? path.join(web, "server.js")
  : path.join(web, ".next", "standalone", "server.js");

// Docker sets HOSTNAME to the container ID unless overridden; Cloud Run needs 0.0.0.0.
process.env.HOSTNAME = "0.0.0.0";

runTogether([
  { command: "song-lingo", args: ["serve"], cwd: web },
  { command: process.execPath, args: [server], cwd: path.dirname(server) },
]);
