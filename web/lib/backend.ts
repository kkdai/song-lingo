// Server-side access to the Go API (backend/). In the browser, /api/* reaches it through the
// fallback rewrite in next.config.ts instead.

export const BACKEND_URL = process.env.BACKEND_URL ?? "http://127.0.0.1:8081";

/** GET a JSON endpoint of the Go API; null when it answers 404. */
export async function backendGet<T>(path: string): Promise<T | null> {
  const res = await fetch(`${BACKEND_URL}${path}`, { cache: "no-store" });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`Go API ${path} 回傳 ${res.status}：${await res.text()}`);
  return res.json();
}
