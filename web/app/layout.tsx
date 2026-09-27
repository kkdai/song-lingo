import type { Metadata, Viewport } from "next";
import { THEME_COLOR } from "@/lib/appIcon";
import "./globals.css";

export const metadata: Metadata = {
  title: "Song Lingo",
  description: "用喜歡的歌學語言：逐句拼音、翻譯、文法，還有老師示範發音",
  appleWebApp: { capable: true, title: "Song Lingo", statusBarStyle: "default" },
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  // Draw under the iPhone notch / home indicator; pages pad themselves with safe-area insets.
  viewportFit: "cover",
  themeColor: THEME_COLOR,
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="zh-Hant" className="h-full antialiased">
      <head>
        {/* Not metadata.manifest: that link omits credentials, and IAP would redirect the manifest request to login. */}
        <link rel="manifest" href="/pwa/manifest" crossOrigin="use-credentials" />
      </head>
      <body className="min-h-full flex flex-col">{children}</body>
    </html>
  );
}
