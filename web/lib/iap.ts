import { createRemoteJWKSet, jwtVerify, type JWTVerifyGetKey } from "jose";

// https://cloud.google.com/iap/docs/signed-headers-howto
const IAP_ISSUER = "https://cloud.google.com/iap";
const IAP_KEYS = createRemoteJWKSet(new URL("https://www.gstatic.com/iap/verify/public_key-jwk"));

export type AuthResult = { ok: true; email: string } | { ok: false; status: 401 | 403 | 500; reason: string };

/**
 * Second line of defense behind IAP: verify the signed header IAP adds to every request and
 * check the caller against an allowlist. Fails closed when not configured.
 */
export async function checkIapAssertion(
  assertion: string | null,
  config: { audience?: string; allowedEmails?: string },
  keys: JWTVerifyGetKey = IAP_KEYS,
): Promise<AuthResult> {
  const allowed = (config.allowedEmails ?? "")
    .split(",")
    .map((e) => e.trim().toLowerCase())
    .filter(Boolean);
  if (!config.audience || allowed.length === 0) {
    return { ok: false, status: 500, reason: "Access control is not configured." };
  }
  if (!assertion) return { ok: false, status: 401, reason: "Missing IAP assertion." };

  let email: unknown;
  try {
    const { payload } = await jwtVerify(assertion, keys, {
      issuer: IAP_ISSUER,
      audience: config.audience,
      algorithms: ["ES256"],
    });
    email = payload.email;
  } catch {
    return { ok: false, status: 401, reason: "Invalid IAP assertion." };
  }

  if (typeof email !== "string" || !allowed.includes(email.toLowerCase())) {
    return { ok: false, status: 403, reason: "Forbidden." };
  }
  return { ok: true, email };
}
