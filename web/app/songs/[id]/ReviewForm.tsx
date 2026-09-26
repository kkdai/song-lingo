"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import type { Line } from "@/lib/songs";

export async function saveEdit(songId: string, index: number, edit: Record<string, unknown>): Promise<string | null> {
  const res = await fetch(`/api/songs/${songId}/lines/${index}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(edit),
  });
  if (res.ok) return null;
  const { error } = await res.json().catch(() => ({ error: null }));
  return error ?? `儲存失敗（${res.status}）`;
}

export default function ReviewForm(props: {
  songId: string;
  index: number;
  line: Line;
  language: string;
  sameCount: number;
  onDone: () => void;
}) {
  const { line } = props;
  const router = useRouter();
  const [text, setText] = useState(line.text);
  const [reading, setReading] = useState(line.reading ?? "");
  const [translation, setTranslation] = useState(line.translation_zh ?? "");
  const [applyToSame, setApplyToSame] = useState(props.sameCount > 1);
  const [reviewed, setReviewed] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    setError(null);
    const err = await saveEdit(props.songId, props.index, {
      text,
      ...(props.language === "ja" && { reading }),
      translation_zh: translation,
      reviewed,
      applyToSame,
    });
    setSaving(false);
    if (err) return setError(err);
    router.refresh();
    props.onDone();
  };

  const field = "w-full rounded-md border border-stone-300 bg-white px-2 py-1.5 dark:border-stone-700 dark:bg-stone-950";

  return (
    <form onSubmit={submit} className="mt-4 flex flex-col gap-3 rounded-lg bg-stone-50 p-4 text-sm dark:bg-stone-950/60">
      <label className="flex flex-col gap-1">
        <span className="font-semibold text-stone-500">歌詞原文</span>
        <textarea value={text} onChange={(e) => setText(e.target.value)} rows={2} className={`${field} text-lg`} />
      </label>
      {props.language === "ja" && (
        <label className="flex flex-col gap-1">
          <span className="font-semibold text-stone-500">讀音（平假名）</span>
          <input value={reading} onChange={(e) => setReading(e.target.value)} className={field} />
        </label>
      )}
      <label className="flex flex-col gap-1">
        <span className="font-semibold text-stone-500">中文翻譯</span>
        <input value={translation} onChange={(e) => setTranslation(e.target.value)} className={field} />
      </label>
      {props.sameCount > 1 && (
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={applyToSame} onChange={(e) => setApplyToSame(e.target.checked)} />
          套用到所有相同的歌詞（共 {props.sameCount} 處）
        </label>
      )}
      <label className="flex items-center gap-2">
        <input type="checkbox" checked={reviewed} onChange={(e) => setReviewed(e.target.checked)} />
        標記為已校對
      </label>
      <p className="text-xs text-stone-500">
        修改原文或讀音後，拼音與單字拆解需要「重新分析」才會更新；老師示範音會在下次播放時重新生成。
      </p>
      {error && <p className="rounded-md bg-rose-50 px-3 py-2 text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">{error}</p>}
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={saving || !text.trim()}
          className="rounded-full bg-amber-500 px-4 py-1.5 font-medium text-white hover:bg-amber-600 disabled:opacity-40"
        >
          {saving ? "儲存中…" : "儲存"}
        </button>
        <button type="button" onClick={props.onDone} className="rounded-full px-4 py-1.5 hover:bg-stone-200 dark:hover:bg-stone-800">
          取消
        </button>
      </div>
    </form>
  );
}
