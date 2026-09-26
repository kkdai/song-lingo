import { NextResponse, type NextRequest } from "next/server";
import { checkIapAssertion } from "@/lib/iap";

/**
 * On Cloud Run (K_SERVICE is set by the platform) every request must carry a valid IAP assertion
 * for an allowlisted account — even though IAP already sits in front, so a misconfigured IAP or
 * an accidental --allow-unauthenticated still can't expose the lyrics or spend the Gemini quota.
 * Local development (no K_SERVICE) is unaffected.
 */
export async function proxy(request: NextRequest) {
  if (!process.env.K_SERVICE) return NextResponse.next();

  const result = await checkIapAssertion(request.headers.get("x-goog-iap-jwt-assertion"), {
    audience: process.env.IAP_AUDIENCE,
    allowedEmails: process.env.ALLOWED_EMAILS,
  });
  if (!result.ok) {
    console.warn(`[auth] rejected ${request.method} ${request.nextUrl.pathname}: ${result.reason}`);
    return new NextResponse(result.reason, { status: result.status });
  }
  return NextResponse.next();
}
