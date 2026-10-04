import type { NextRequest } from "next/server";
import { setLearned, updateProgress } from "@/lib/progress";
import { isVideoId, readSong } from "@/lib/songs";

function fail(status: number, error: string) {
  return Response.json({ error }, { status });
}

/**
 * Save study progress: `lastLine` (where to resume) and/or `learned: { index, value }`.
 * Learned status is stored by the line's text, so every repeat of a chorus line follows.
 */
export async function PATCH(request: NextRequest, ctx: RouteContext<"/api/songs/[id]/progress">) {
  const { id } = await ctx.params;
  if (!isVideoId(id)) return fail(404, "Not found");
  const song = await readSong(id);
  if (!song) return fail(404, "找不到這首歌。");

  const body = await request.json().catch(() => null);
  if (!body || typeof body !== "object") return fail(400, "請求格式不正確。");
  const validIndex = (i: unknown): i is number => Number.isInteger(i) && (i as number) >= 0 && (i as number) < song.lines.length;

  const { lastLine, learned } = body as { lastLine?: unknown; learned?: { index?: unknown; value?: unknown } };
  if (lastLine !== undefined && !validIndex(lastLine)) return fail(400, "lastLine 格式不正確。");
  if (learned !== undefined && (!validIndex(learned?.index) || typeof learned.value !== "boolean")) {
    return fail(400, "learned 格式不正確。");
  }

  await updateProgress(id, (p) => {
    if (lastLine !== undefined) p.lastLine = lastLine;
    if (learned) setLearned(p, song.lines[learned.index as number].text, learned.value as boolean);
  });
  return new Response(null, { status: 204 });
}
