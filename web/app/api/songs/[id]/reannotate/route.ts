import { ReviewError, reannotate } from "@/lib/review";
import { isVideoId } from "@/lib/songs";

export async function POST(_request: Request, ctx: RouteContext<"/api/songs/[id]/reannotate">) {
  const { id } = await ctx.params;
  if (!isVideoId(id)) return Response.json({ error: "Not found" }, { status: 404 });
  try {
    return Response.json({ summary: await reannotate(id) });
  } catch (e) {
    if (e instanceof ReviewError) return Response.json({ error: e.message }, { status: e.status });
    throw e;
  }
}
