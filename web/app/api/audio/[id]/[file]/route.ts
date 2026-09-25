import { readFile } from "node:fs/promises";
import path from "node:path";
import type { NextRequest } from "next/server";
import { DATA_DIR, isVideoId } from "@/lib/songs";

const CLIP_FILE = /^\d{3}_(normal|slow)\.wav$/;

export async function GET(request: NextRequest, ctx: RouteContext<"/api/audio/[id]/[file]">) {
  const { id, file } = await ctx.params;
  if (!isVideoId(id) || !CLIP_FILE.test(file)) {
    return new Response("Not found", { status: 404 });
  }

  let data: Buffer;
  try {
    data = await readFile(path.join(DATA_DIR, "audio", id, file));
  } catch {
    return new Response("Not found", { status: 404 });
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
