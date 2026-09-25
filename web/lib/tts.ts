import { existsSync, readFileSync } from "node:fs";
import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { DATA_DIR, readSong, uniqueTexts } from "@/lib/songs";
import { CLIP_FILE, CONFIG, REPO_ROOT } from "@/lib/teachers";

const API = "https://generativelanguage.googleapis.com/v1beta";
const REQUEST_TIMEOUT_MS = 60_000;
const VOICES_FILE = path.join(DATA_DIR, "voices.json");

export class TtsError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}

function apiKey(): string {
  // The key lives in the repo-root .env shared with the Python scripts.
  let key = process.env.GEMINI_API_KEY;
  const envFile = path.join(REPO_ROOT, ".env");
  if (!key && existsSync(envFile)) {
    // Accept the same forms python-dotenv does: optional `export`, optional quotes.
    const raw = readFileSync(envFile, "utf8").match(/^\s*(?:export\s+)?GEMINI_API_KEY\s*=\s*(.*)$/m)?.[1]?.trim();
    key = raw?.replace(/^(['"])(.*)\1$/, "$2");
  }
  if (!key) throw new TtsError(500, "找不到 GEMINI_API_KEY，請在專案根目錄的 .env 設定。");
  return key;
}

async function callApi(method: string, endpoint: string, body?: unknown) {
  let res: Response;
  try {
    res = await fetch(`${API}/${endpoint}`, {
      method,
      headers: { "x-goog-api-key": apiKey(), "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
      signal: AbortSignal.timeout(REQUEST_TIMEOUT_MS),
    });
  } catch (e) {
    const timedOut = e instanceof DOMException && e.name === "TimeoutError";
    throw new TtsError(504, timedOut ? "Gemini TTS 回應逾時，請再試一次。" : `無法連線到 Gemini：${e}`);
  }
  const json = await res.json().catch(() => ({}));
  if (!res.ok) {
    // Errors come back either as {error} or as [{error}].
    const message = (Array.isArray(json) ? json[0] : json)?.error?.message ?? res.statusText;
    if (res.status === 429) throw new TtsError(429, `TTS 額度已用完：${message}`);
    throw new TtsError(502, `Gemini 回傳錯誤 ${res.status}：${message}`);
  }
  return json;
}

const pendingVoices = new Map<string, Promise<string>>();

/** Voice-design one teacher voice per language, cached in output/voices.json. */
async function teacherVoice(language: string): Promise<string> {
  const voices: Record<string, string> = existsSync(VOICES_FILE) ? JSON.parse(await readFile(VOICES_FILE, "utf8")) : {};
  if (voices[language]) return voices[language];
  const teacher = CONFIG.teachers[language];
  if (!teacher) throw new TtsError(400, `還沒有「${language}」的老師聲音設定。`);

  let pending = pendingVoices.get(language);
  if (!pending) {
    pending = (async () => {
      const created = await callApi("POST", "voices", {
        voice: {
          type: "prompted",
          display_name: teacher.display_name,
          gender: "female",
          language_code: teacher.language_code,
          model: CONFIG.model,
          prompted: { input: teacher.description },
        },
        store: true,
      });
      const latest = existsSync(VOICES_FILE) ? JSON.parse(await readFile(VOICES_FILE, "utf8")) : {};
      await writeFile(VOICES_FILE, JSON.stringify({ ...latest, [language]: created.id }, null, 2));
      return created.id as string;
    })().finally(() => pendingVoices.delete(language));
    pendingVoices.set(language, pending);
  }
  return pending;
}

/** Wrap raw 16-bit mono PCM in a WAV header (in case the API returns headerless audio). */
function toWav(audio: Buffer, sampleRate = 24_000): Buffer {
  if (audio.subarray(0, 4).toString("ascii") === "RIFF") return audio;
  const header = Buffer.alloc(44);
  header.write("RIFF", 0, "ascii");
  header.writeUInt32LE(36 + audio.length, 4);
  header.write("WAVEfmt ", 8, "ascii");
  header.writeUInt32LE(16, 16);
  header.writeUInt16LE(1, 20); // PCM
  header.writeUInt16LE(1, 22); // mono
  header.writeUInt32LE(sampleRate, 24);
  header.writeUInt32LE(sampleRate * 2, 28);
  header.writeUInt16LE(2, 32);
  header.writeUInt16LE(16, 34);
  header.write("data", 36, "ascii");
  header.writeUInt32LE(audio.length, 40);
  return Buffer.concat([header, audio]);
}

async function synthesize(text: string, voice: string, style: string): Promise<Buffer> {
  const json = await callApi("POST", "interactions", {
    model: CONFIG.model,
    input: [
      {
        type: "user_input",
        content: [{ type: "text", text, annotations: [{ type: "speech_metadata", style }] }],
      },
    ],
    response_format: { type: "audio" },
    generation_config: { speech_config: [{ voice }] },
  });
  const data = json.output_audio?.data ?? json.outputAudio?.data;
  if (!data) throw new TtsError(502, "Gemini 沒有回傳音訊。");
  return toWav(Buffer.from(data, "base64"));
}

const pendingClips = new Map<string, Promise<Buffer>>();

/** Return a teacher clip, generating and caching it on first request. */
export async function getClip(videoId: string, file: string): Promise<Buffer> {
  const match = file.match(CLIP_FILE);
  if (!match) throw new TtsError(404, "Not found");
  const clipPath = path.join(DATA_DIR, "audio", videoId, file);
  if (existsSync(clipPath)) return readFile(clipPath);

  // Safari fires several range requests at once; share one generation between them.
  let pending = pendingClips.get(clipPath);
  if (!pending) {
    pending = (async () => {
      const song = await readSong(videoId);
      const text = song && uniqueTexts(song)[Number(match[1])];
      if (!song || !text) throw new TtsError(404, "Not found");
      const voice = await teacherVoice(song.language);
      const audio = await synthesize(text, voice, CONFIG.styles[match[2]]);
      await mkdir(path.dirname(clipPath), { recursive: true });
      await writeFile(`${clipPath}.tmp`, audio);
      await rename(`${clipPath}.tmp`, clipPath);
      return audio;
    })().finally(() => pendingClips.delete(clipPath));
    pendingClips.set(clipPath, pending);
  }
  return pending;
}
