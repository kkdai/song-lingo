import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { DATA_DIR, type Line } from "@/lib/songs";

/** Attempts kept per line; older ones are dropped. */
const MAX_ATTEMPTS = 20;

export type ShadowAttempt = {
  at: string;
  score: number;
  /** Words that weren't heard as written (for "words I keep missing"). */
  missed: string[];
};

export type LineProgress = {
  learned?: boolean;
  shadow?: ShadowAttempt[];
};

/**
 * What the learner has done in one song. Lines are keyed by their text, like the teacher-audio
 * cache: repeated choruses share one entry, and an edit to a line's text carries its entry over.
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

function withDefaults(data: Partial<Progress> | undefined): Progress {
  return { ...emptyProgress(), ...data };
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
    try {
      return withDefaults(JSON.parse(await readFile(progressFile(videoId), "utf8")));
    } catch {
      return emptyProgress();
    }
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

function entry(progress: Progress, text: string): LineProgress {
  return (progress.lines[text] ??= {});
}

export function setLearned(progress: Progress, text: string, learned: boolean) {
  if (learned) entry(progress, text).learned = true;
  else delete entry(progress, text).learned;
}

export function addAttempt(progress: Progress, text: string, attempt: ShadowAttempt) {
  const e = entry(progress, text);
  e.shadow = [...(e.shadow ?? []), attempt].slice(-MAX_ATTEMPTS);
}

/** Carry a line's progress over when the review UI changes its text. */
export async function renameLine(videoId: string, from: string, to: string) {
  if (from === to || !(await readProgress(videoId)).lines[from]) return;
  await updateProgress(videoId, (p) => {
    // Keep the old entry too: other lines with the old text may not have been edited.
    if (p.lines[from] && !p.lines[to]) p.lines[to] = structuredClone(p.lines[from]);
  });
}

export function summarize(progress: Progress, lines: Line[]): ProgressSummary {
  return {
    learned: lines.filter((l) => progress.lines[l.text]?.learned).length,
    total: lines.length,
    lastLine: progress.lastLine,
    lastStudiedAt: progress.lastStudiedAt,
  };
}
