import type { NextRequest } from "next/server";
import { isVideoId } from "@/lib/songs";
import { CLIP_FILE } from "@/lib/teachers";
import { TtsError, getClip } from "@/lib/tts";

// Last generation failure per clip, so the page can explain an <audio> error
// (media elements never expose the response body) without triggering another TTS request.
const recentErrors = new Map<string, { status: number; message: string; at: number }>();
const ERROR_TTL_MS = 60_000;

export async function GET(request: NextRequest, ctx: RouteContext<"/api/audio/[id]/[file]">) {
  const { id, file } = await ctx.params;
  if (!isVideoId(id) || !CLIP_FILE.test(file)) {
    return Response.json({ error: "Not found" }, { status: 404 });
  }
  const key = `${id}/${file}`;

  if (request.nextUrl.searchParams.has("error")) {
    const last = recentErrors.get(key);
    const fresh = last && Date.now() - last.at < ERROR_TTL_MS;
    return Response.json({ error: fresh ? last.message : null }, { status: fresh ? last.status : 200 });
  }

  let data: Buffer;
  try {
    data = await getClip(id, file);
    recentErrors.delete(key);
  } catch (e) {
    if (!(e instanceof TtsError)) throw e;
    recentErrors.set(key, { status: e.status, message: e.message, at: Date.now() });
    return Response.json({ error: e.message }, { status: e.status });
  }

  const headers = {
    "Content-Type": "audio/wav",
    "Accept-Ranges": "bytes",
    "Cache-Control": "private, max-age=3600",
  };

  // Safari requires byte-range support to play media.
  const range = request.headers.get("range")?.match(/^bytes=(\d*)-(\d*)$/);
  if (range) {
    const size = data.length;
    const start = range[1] ? Number(range[1]) : Math.max(0, size - Number(range[2]));
    const end = range[1] && range[2] ? Math.min(Number(range[2]), size - 1) : size - 1;
    if (start > end || start >= size) {
      return new Response(null, { status: 416, headers: { "Content-Range": `bytes */${size}` } });
    }
    return new Response(new Uint8Array(data.subarray(start, end + 1)), {
      status: 206,
      headers: { ...headers, "Content-Range": `bytes ${start}-${end}/${size}`, "Content-Length": String(end - start + 1) },
    });
  }

  return new Response(new Uint8Array(data), { headers: { ...headers, "Content-Length": String(data.length) } });
}
