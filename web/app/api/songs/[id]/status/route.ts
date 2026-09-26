import { jobStatus } from "@/lib/pipeline";
import { isVideoId } from "@/lib/songs";

export async function GET(_request: Request, ctx: RouteContext<"/api/songs/[id]/status">) {
  const { id } = await ctx.params;
  const status = isVideoId(id) ? jobStatus(id) : null;
  if (!status) return Response.json({ error: "沒有這首歌的處理紀錄。" }, { status: 404 });
  return Response.json(status);
}
