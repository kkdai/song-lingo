import Link from "next/link";
import { LANGUAGE_LABELS } from "@/lib/labels";
import { listSongs } from "@/lib/songs";

export const dynamic = "force-dynamic";

export default async function Home() {
  const songs = await listSongs();

  return (
    <main className="mx-auto w-full max-w-3xl px-4 py-10">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="text-3xl font-bold">Song Lingo</h1>
          <p className="mt-2 text-stone-500">用喜歡的歌學語言：逐句拼音、翻譯、文法，還有老師示範發音。</p>
        </div>
        <Link href="/add" className="rounded-full bg-amber-500 px-4 py-2 text-sm font-medium text-white hover:bg-amber-600">
          ＋ 加入新歌
        </Link>
      </div>

      {songs.length === 0 ? (
        <div className="mt-8 rounded-xl border border-dashed border-stone-300 p-6 text-sm text-stone-500 dark:border-stone-700">
          還沒有歌曲。按右上角「＋ 加入新歌」，貼上 MV 的 YouTube 網址就可以開始。
        </div>
      ) : (
        <ul className="mt-8 grid gap-3 sm:grid-cols-2">
          {songs.map((song) => (
            <li key={song.id}>
              <Link
                href={`/songs/${song.id}`}
                className="flex gap-3 rounded-xl border border-stone-200 bg-white p-3 transition hover:border-stone-400 hover:shadow-sm dark:border-stone-800 dark:bg-stone-900 dark:hover:border-stone-600"
              >
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={`https://i.ytimg.com/vi/${song.id}/mqdefault.jpg`}
                  alt=""
                  className="aspect-video w-32 shrink-0 rounded-md object-cover"
                />
                <div className="min-w-0">
                  <div className="truncate font-semibold">{song.title_guess ?? song.id}</div>
                  <div className="truncate text-sm text-stone-500">{song.artist_guess ?? "未知歌手"}</div>
                  <div className="mt-2 flex flex-wrap gap-1.5 text-xs">
                    <span className="rounded-full bg-amber-100 px-2 py-0.5 text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">
                      {LANGUAGE_LABELS[song.language] ?? song.language}
                    </span>
                    <span className="rounded-full bg-stone-100 px-2 py-0.5 text-stone-600 dark:bg-stone-800 dark:text-stone-300">
                      {song.lineCount} 句
                    </span>
                    {song.hasTeacher && (
                      <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-emerald-800 dark:bg-emerald-900/40 dark:text-emerald-200">
                        🔊 老師示範
                      </span>
                    )}
                  </div>
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
