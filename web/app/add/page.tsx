import Link from "next/link";
import AddSongForm from "./AddSongForm";

export default function AddSongPage() {
  return (
    <main className="mx-auto w-full max-w-2xl px-4 py-10">
      <Link href="/" className="text-sm text-stone-500 hover:text-stone-800 dark:hover:text-stone-200">
        ← 歌曲列表
      </Link>
      <h1 className="mt-2 text-2xl font-bold">加入新歌</h1>
      <p className="mb-6 mt-1 text-stone-500">貼上 MV 的 YouTube 網址，自動轉錄歌詞並產生逐句教學。</p>
      <AddSongForm />
    </main>
  );
}
