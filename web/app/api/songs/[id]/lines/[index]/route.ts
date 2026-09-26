import type { NextRequest } from "next/server";
import { ReviewError, editLine, parseEdit } from "@/lib/review";
import { isVideoId } from "@/lib/songs";

export async function PATCH(request: NextRequest, ctx: RouteContext<"/api/songs/[id]/lines/[index]">) {
  const { id, index } = await ctx.params;
  if (!isVideoId(id) || !/^\d+$/.test(index)) return Response.json({ error: "Not found" }, { status: 404 });
  try {
    const edit = parseEdit(await request.json().catch(() => null));
    const updated = await editLine(id, Number(index), edit);
    return Response.json({ updated });
  } catch (e) {
    if (e instanceof ReviewError) return Response.json({ error: e.message }, { status: e.status });
    throw e;
  }
}
