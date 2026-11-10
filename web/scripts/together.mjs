// Run the Go API and Next.js side by side. If either exits (or on Ctrl-C / SIGTERM), stop the
// other and exit too, so a container platform restarts the whole thing.
import { spawn } from "node:child_process";

/** @param {{ command: string, args: string[], cwd: string }[]} commands */
export function runTogether(commands) {
  const children = commands.map(({ command, args, cwd }) => spawn(command, args, { cwd, stdio: "inherit" }));
  let exiting = false;
  const stopAll = (code) => {
    if (exiting) return;
    exiting = true;
    for (const child of children) if (child.exitCode === null && child.signalCode === null) child.kill("SIGTERM");
    process.exitCode = code;
  };
  for (const child of children) {
    child.on("exit", (code) => stopAll(code ?? 1));
    child.on("error", (err) => {
      console.error(err.message);
      stopAll(1);
    });
  }
  for (const signal of ["SIGINT", "SIGTERM"]) process.on(signal, () => stopAll(0));
}
