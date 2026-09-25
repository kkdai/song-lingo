import Link from "next/link";
import { notFound } from "next/navigation";
import { LANGUAGE_LABELS } from "@/lib/labels";
import { getSong } from "@/lib/songs";
import SongStudy from "./SongStudy";

export const dynamic = "force-dynamic";

export default async function SongPage({ params }: PageProps<"/songs/[id]">) {
  const { id } = await params;
  const song = await getSong(id);
  if (!song) notFound();

  return (
    <main className="mx-auto w-full max-w-7xl px-4 py-6">
      <header className="mb-5 flex flex-wrap items-baseline gap-x-3 gap-y-1">
        <Link href="/" className="text-sm text-stone-500 hover:text-stone-800 dark:hover:text-stone-200">
          ← 歌曲列表
        </Link>
        <h1 className="text-2xl font-bold">{song.title_guess ?? song.id}</h1>
        <span className="text-stone-500">{song.artist_guess}</span>
        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs text-amber-800 dark:bg-amber-900/40 dark:text-amber-200">
          {LANGUAGE_LABELS[song.language] ?? song.language}
        </span>
      </header>
      <SongStudy song={song} />
    </main>
  );
}
