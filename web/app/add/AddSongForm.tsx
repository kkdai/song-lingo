"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";

type Stage = "transcribing" | "annotating" | "done" | "error";
type Job = { id: string; stage: Stage; message?: string; startedAt?: number };

const STEPS: { stage: Stage; label: string; hint: string }[] = [
  { stage: "transcribing", label: "轉錄歌詞", hint: "Gemini 聽 MV、讀畫面字幕，約 30–90 秒" },
  { stage: "annotating", label: "拼音、翻譯與文法", hint: "約 10–30 秒" },
];

export default function AddSongForm() {
  const router = useRouter();
  const [url, setUrl] = useState("");
  const [job, setJob] = useState<Job | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [now, setNow] = useState(() => Date.now());

  const busy = job !== null && (job.stage === "transcribing" || job.stage === "annotating");

  // Poll the background job until it finishes.
  useEffect(() => {
    if (!job || !busy) return;
    const timer = setInterval(async () => {
      setNow(Date.now());
      const res = await fetch(`/api/songs/${job.id}/status`).catch(() => null);
      if (res?.ok) setJob(await res.json());
    }, 2000);
    return () => clearInterval(timer);
  }, [job, busy]);

  useEffect(() => {
    if (job?.stage === "done") {
      router.push(`/songs/${job.id}`);
      router.refresh();
    }
  }, [job, router]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    const res = await fetch("/api/songs", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ url }),
    }).catch(() => null);
    setSubmitting(false);
    const body = await res?.json().catch(() => null);
    if (!res?.ok) return setError(body?.error ?? "無法開始處理，請確認開發伺服器還在執行。");
    setNow(Date.now());
    setJob(body);
  };

  const currentStep = STEPS.findIndex((s) => s.stage === job?.stage);
  const elapsed = job?.startedAt ? Math.max(0, Math.round((now - job.startedAt) / 1000)) : 0;

  return (
    <div className="flex flex-col gap-6">
      <form onSubmit={submit} className="flex flex-col gap-3 sm:flex-row">
        <input
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://www.youtube.com/watch?v=..."
          disabled={busy}
          className="min-w-0 flex-1 rounded-full border border-stone-300 bg-white px-4 py-2 dark:border-stone-700 dark:bg-stone-900"
        />
        <button
          type="submit"
          disabled={!url.trim() || busy || submitting}
          className="rounded-full bg-amber-500 px-5 py-2 font-medium text-white hover:bg-amber-600 disabled:opacity-40"
        >
          {submitting ? "送出中…" : "加入"}
        </button>
      </form>

      {error && (
        <p className="rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">{error}</p>
      )}

      {job && (
        <div className="rounded-xl border border-stone-200 bg-white p-5 dark:border-stone-800 dark:bg-stone-900">
          <div className="mb-4 flex gap-3">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={`https://i.ytimg.com/vi/${job.id}/mqdefault.jpg`} alt="" className="aspect-video w-32 rounded-md object-cover" />
            <div className="text-sm text-stone-500">
              <div className="font-mono">{job.id}</div>
              {busy && <div className="mt-1">已經過 {elapsed} 秒，可以先離開這頁，處理會在背景繼續。</div>}
            </div>
          </div>
          <ol className="flex flex-col gap-2">
            {STEPS.map((step, i) => {
              const done = job.stage === "done" || (currentStep > i && job.stage !== "error");
              const active = currentStep === i && busy;
              return (
                <li key={step.stage} className="flex items-center gap-2 text-sm">
                  <span className="w-5 text-center">{done ? "✅" : active ? "⏳" : "○"}</span>
                  <span className={active ? "font-semibold" : done ? "" : "text-stone-400"}>{step.label}</span>
                  {active && <span className="text-xs text-stone-500">{step.hint}</span>}
                </li>
              );
            })}
          </ol>
          {job.stage === "done" && <p className="mt-3 text-sm text-emerald-700">完成！正在前往學習頁面…</p>}
          {job.stage === "error" && (
            <div className="mt-3 rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
              <p className="break-words">{job.message}</p>
              <p className="mt-1 text-xs">再按一次「加入」會重試；已轉錄完成的部分不會重做。</p>
            </div>
          )}
        </div>
      )}

      <p className="text-xs leading-relaxed text-stone-500">
        只能處理公開的 YouTube 影片。MV 畫面上有歌詞字幕時，轉錄會準確很多。一首歌約使用 2 次 Gemini Flash 請求（不佔 TTS 額度）；老師示範音會在學習時才產生。
        歌詞受著作權保護，只存在這台電腦的 output/ 資料夾，供個人學習使用。
      </p>
    </div>
  );
}
