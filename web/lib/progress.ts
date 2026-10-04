import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { type LineProgress, type ShadowAttempt, withAttempt } from "@/lib/progressShared";
import { DATA_DIR, type Line } from "@/lib/songs";
import { textHash } from "@/lib/teachers";

export type { LineProgress, ShadowAttempt } from "@/lib/progressShared";

/**
 * What the learner has done in one song. Lines are keyed by a hash of their text (`lineKey`), like
 * the teacher-audio cache: repeated choruses share one entry, an edit to a line's text carries its
 * entry over, and no lyrics are copied into the progress store.
 */
export type Progress = {
  version: 1;
  lastLine: number;
  lastStudiedAt: string | null;
  lines: Record<string, LineProgress>;
};

export type ProgressSummary = {
  learned: number;
  total: number;
  lastLine: number;
  lastStudiedAt: string | null;
};

function emptyProgress(): Progress {
  return { version: 1, lastLine: 0, lastStudiedAt: null, lines: {} };
}

function progressFile(videoId: string): string {
  return path.join(DATA_DIR, "progress", `${videoId}.json`);
}

export const lineKey = textHash;
const LINE_KEY = /^[0-9a-f]{16}$/;

function withDefaults(data: Partial<Progress> | undefined): Progress {
  const progress = { ...emptyProgress(), ...data };
  // Early versions keyed lines by their raw text; re-key those entries by hash.
  for (const [key, value] of Object.entries(progress.lines)) {
    if (LINE_KEY.test(key)) continue;
    delete progress.lines[key];
    const existing = progress.lines[lineKey(key)];
    progress.lines[lineKey(key)] = {
      ...(existing?.learned || value.learned ? { learned: true } : {}),
      shadow: [...(existing?.shadow ?? []), ...(value.shadow ?? [])].sort((a, b) => a.at.localeCompare(b.at)),
    };
  }
  return progress;
}

function stamp(progress: Progress, change: (p: Progress) => void): Progress {
  change(progress);
  progress.lastStudiedAt = new Date().toISOString();
  return progress;
}

/** Where progress lives: Firestore when FIRESTORE_DATABASE is set (Cloud Run), else JSON files. */
type Store = {
  read(videoId: string): Promise<Progress>;
  update(videoId: string, change: (p: Progress) => void): Promise<Progress>;
};

/** Local development: output/progress/<videoId>.json, writes serialized per song in memory. */
function fileStore(): Store {
  const queues = new Map<string, Promise<unknown>>();
  const read = async (videoId: string) => {
    let raw: string;
    try {
      raw = await readFile(progressFile(videoId), "utf8");
    } catch (e) {
      // Only a missing file means "no progress yet"; any other failure must not be saved over.
      if ((e as NodeJS.ErrnoException).code === "ENOENT") return emptyProgress();
      throw e;
    }
    return withDefaults(JSON.parse(raw));
  };
  return {
    read,
    update(videoId, change) {
      const run = (queues.get(videoId) ?? Promise.resolve()).catch(() => {}).then(async () => {
        const progress = stamp(await read(videoId), change);
        const file = progressFile(videoId);
        await mkdir(path.dirname(file), { recursive: true });
        await writeFile(`${file}.tmp`, JSON.stringify(progress, null, 2));
        await rename(`${file}.tmp`, file);
        return progress;
      });
      queues.set(videoId, run);
      run.finally(() => queues.get(videoId) === run && queues.delete(videoId)).catch(() => {});
      return run;
    },
  };
}

/**
 * Cloud Run: one document per song in the `progress` collection of a dedicated Firestore
 * database. Updates run in a transaction, so they stay safe with more than one instance.
 */
async function firestoreStore(databaseId: string): Promise<Store> {
  const { Firestore } = await import("@google-cloud/firestore");
  const db = new Firestore({ databaseId, ignoreUndefinedProperties: true });
  const doc = (videoId: string) => db.collection("progress").doc(videoId);
  return {
    async read(videoId) {
      return withDefaults((await doc(videoId).get()).data() as Partial<Progress> | undefined);
    },
    update(videoId, change) {
      return db.runTransaction(async (tx) => {
        const ref = doc(videoId);
        const progress = stamp(withDefaults((await tx.get(ref)).data() as Partial<Progress> | undefined), change);
        tx.set(ref, progress);
        return progress;
      });
    },
  };
}

let store: Promise<Store> | undefined;
function getStore(): Promise<Store> {
  const databaseId = process.env.FIRESTORE_DATABASE;
  return (store ??= databaseId ? firestoreStore(databaseId) : Promise.resolve(fileStore()));
}

export async function readProgress(videoId: string): Promise<Progress> {
  return (await getStore()).read(videoId);
}

/** Read-modify-write the song's progress atomically; also records when it was last studied. */
export async function updateProgress(videoId: string, change: (p: Progress) => void): Promise<Progress> {
  return (await getStore()).update(videoId, change);
}

export function setLearned(progress: Progress, text: string, learned: boolean) {
  const entry = (progress.lines[lineKey(text)] ??= {});
  if (learned) entry.learned = true;
  else delete entry.learned;
}

export function addAttempt(progress: Progress, text: string, attempt: ShadowAttempt) {
  progress.lines[lineKey(text)] = withAttempt(progress.lines[lineKey(text)], attempt);
}

/** Carry a line's progress over when the review UI changes its text. */
export async function renameLine(videoId: string, from: string, to: string) {
  const [fromKey, toKey] = [lineKey(from), lineKey(to)];
  if (fromKey === toKey || !(await readProgress(videoId)).lines[fromKey]) return;
  await updateProgress(videoId, (p) => {
    // Keep the old entry too: other lines with the old text may not have been edited.
    if (p.lines[fromKey] && !p.lines[toKey]) p.lines[toKey] = structuredClone(p.lines[fromKey]);
  });
}

export function summarize(progress: Progress, lines: Line[]): ProgressSummary {
  return {
    learned: lines.filter((l) => progress.lines[lineKey(l.text)]?.learned).length,
    total: lines.length,
    lastLine: progress.lastLine,
    lastStudiedAt: progress.lastStudiedAt,
  };
}
