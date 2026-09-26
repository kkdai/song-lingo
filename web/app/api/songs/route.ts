import { PipelineError, addSong } from "@/lib/pipeline";

/** Start adding a song from a YouTube URL; poll /api/songs/[id]/status for progress. */
export async function POST(request: Request) {
  const body = await request.json().catch(() => null);
  if (typeof body?.url !== "string") return Response.json({ error: "請提供 YouTube 網址。" }, { status: 400 });
  try {
    return Response.json(addSong(body.url), { status: 202 });
  } catch (e) {
    if (e instanceof PipelineError) return Response.json({ error: e.message }, { status: e.status });
    throw e;
  }
}
