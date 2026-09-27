import { THEME_COLOR, renderAppIcon } from "@/lib/appIcon";

/**
 * Served from a plain route (not app/manifest.ts) so the layout can link it with
 * crossorigin="use-credentials": browsers fetch manifests without cookies by default, and on
 * Cloud Run IAP would answer that with a login redirect. Icons are inlined as data URLs for the
 * same reason — no separate, cookie-less icon requests.
 */
export const dynamic = "force-static";

async function dataUrl(size: number, maskable = false) {
  const png = Buffer.from(await renderAppIcon(size, { maskable }).arrayBuffer());
  return `data:image/png;base64,${png.toString("base64")}`;
}

export async function GET() {
  const manifest = {
    name: "Song Lingo",
    short_name: "Song Lingo",
    description: "用喜歡的歌學語言：逐句拼音、翻譯、文法，還有老師示範發音。",
    lang: "zh-Hant",
    start_url: "/",
    scope: "/",
    display: "standalone",
    orientation: "portrait",
    background_color: "#fafaf9",
    theme_color: THEME_COLOR,
    icons: [
      { src: await dataUrl(192), sizes: "192x192", type: "image/png", purpose: "any" },
      { src: await dataUrl(512), sizes: "512x512", type: "image/png", purpose: "any" },
      { src: await dataUrl(512, true), sizes: "512x512", type: "image/png", purpose: "maskable" },
    ],
  };
  return new Response(JSON.stringify(manifest), {
    headers: { "Content-Type": "application/manifest+json" },
  });
}
