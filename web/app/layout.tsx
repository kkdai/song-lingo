import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "Song Lingo",
  description: "用歌曲學語言：逐句拼音、翻譯與老師示範發音",
};

export default function RootLayout({ children }: LayoutProps<"/">) {
  return (
    <html lang="zh-Hant" className="h-full antialiased">
      <body className="min-h-full flex flex-col">{children}</body>
    </html>
  );
}
