"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { POS_LABELS, SECTION_LABELS } from "@/lib/labels";
import type { Line, Song, Token } from "@/lib/songs";
import { useYouTubePlayer } from "@/lib/useYouTubePlayer";
import ReviewForm, { saveEdit } from "./ReviewForm";

const KANJI = /[一-鿿々]/;

function toSeconds(timestamp: string): number {
  const [m, s] = timestamp.split(":").map(Number);
  return m * 60 + s;
}

function youtubeUrl(videoId: string, seconds: number): string {
  return `https://www.youtube.com/watch?v=${videoId}&t=${Math.floor(seconds)}s`;
}

const PLAYER_ERRORS: Record<number, string> = {
  101: "影片擁有者不允許在其他網站播放這支影片。",
  150: "影片擁有者不允許在其他網站播放這支影片。",
  100: "找不到這支影片，可能已下架或設為私人。",
  2: "影片網址無效。",
  5: "瀏覽器無法播放這支影片。",
};

/** Shown over the player when YouTube refuses to play the video here. */
function EmbedFallback({ videoId, code, start }: { videoId: string; code: number | null; start: number }) {
  const [thumb, setThumb] = useState(`https://i.ytimg.com/vi/${videoId}/maxresdefault.jpg`);
  return (
    <div className="absolute inset-0">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={thumb}
        alt=""
        // maxresdefault doesn't exist for every video: YouTube then serves a 120x90 gray
        // placeholder (or a 404), so fall back to hqdefault, which always exists.
        onError={() => setThumb(`https://i.ytimg.com/vi/${videoId}/hqdefault.jpg`)}
        onLoad={(e) => {
          if (e.currentTarget.naturalWidth <= 120 && thumb.includes("maxres")) {
            setThumb(`https://i.ytimg.com/vi/${videoId}/hqdefault.jpg`);
          }
        }}
        className="h-full w-full object-cover opacity-60"
      />
      <div className="absolute inset-0 flex flex-col items-center justify-center gap-3 bg-black/40 p-4 text-center text-white">
        <p className="text-sm">{PLAYER_ERRORS[code ?? 0] ?? "播放器載入失敗。"}</p>
        <a
          href={youtubeUrl(videoId, start)}
          target="_blank"
          rel="noopener noreferrer"
          className="rounded-full bg-red-600 px-4 py-2 text-sm font-medium hover:bg-red-700"
        >
          ▶ 在 YouTube 開啟（從這句開始）
        </a>
        <p className="text-xs opacity-80">老師示範音與逐句教學仍可正常使用。</p>
      </div>
    </div>
  );
}

/** A line the learner should double-check: flagged by the pipeline or edited since the last analysis. */
function needsAttention(line: Line): boolean {
  return !line.reviewed && Boolean(line.needs_review || line.uncertain || line.stale);
}

/** Word with furigana when its written form contains kanji. */
function Word({ token }: { token: Token }) {
  if (token.reading && token.reading !== token.surface && KANJI.test(token.surface)) {
    return (
      <ruby>
        {token.surface}
        <rt>{token.reading}</rt>
      </ruby>
    );
  }
  return <>{token.surface}</>;
}

export default function SongStudy({ song }: { song: Song }) {
  const { lines, language } = song;
  const { containerRef, ready, time, error: playerError, playSegment, pause } = useYouTubePlayer(song.id);
  const embedBlocked = playerError !== null;
  const [selected, setSelected] = useState(0);
  const [follow, setFollow] = useState(true);
  const [audioError, setAudioError] = useState<string | null>(null);
  // Clip URLs known to exist on the server; others are generated on first play.
  const [generated, setGenerated] = useState(
    () => new Set(lines.flatMap((l) => (l.audioCached ?? []).map((speed) => l.audio![speed]))),
  );
  const [generating, setGenerating] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const listRef = useRef<HTMLOListElement>(null);

  const spans = useMemo(() => lines.map((l) => [toSeconds(l.start), toSeconds(l.end)] as const), [lines]);

  // Line currently being sung in the MV (null between lines).
  const playing = useMemo(() => {
    for (let i = spans.length - 1; i >= 0; i--) {
      if (time >= spans[i][0]) return time < spans[i][1] + 1 ? i : null;
    }
    return null;
  }, [spans, time]);

  // While following the MV, show the line being sung; otherwise the one the user picked.
  const current = follow && playing !== null ? playing : selected;

  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${current}"]`)
      ?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [current]);

  const pick = useCallback((index: number) => {
    setFollow(false);
    setSelected(index);
  }, []);

  const playTeacher = useCallback(
    (speed: "normal" | "slow") => {
      const src = lines[current]?.audio?.[speed];
      if (!src) return;
      pause();
      setAudioError(null);
      audioRef.current?.pause();
      // Point the element straight at the route and call play() inside the click, so the
      // browser keeps the user gesture even if the server spends seconds generating the clip.
      const audio = new Audio(src);
      audioRef.current = audio;
      if (!generated.has(src)) setGenerating(src);
      audio.addEventListener("playing", () => {
        setGenerating((g) => (g === src ? null : g));
        setGenerated((prev) => (prev.has(src) ? prev : new Set(prev).add(src)));
      });
      audio.addEventListener("error", async () => {
        setGenerating((g) => (g === src ? null : g));
        // <audio> can't read the error body; ask the route what went wrong.
        const { error } = await fetch(`${src}?error=1`).then((r) => r.json()).catch(() => ({ error: null }));
        if (audioRef.current === audio) setAudioError(error ?? "音檔載入失敗，請再試一次。");
      });
      audio.play().catch((e: DOMException) => {
        // AbortError: a newer clip replaced this one. NotSupportedError: reported by the error listener.
        if (e.name !== "AbortError" && e.name !== "NotSupportedError") setAudioError(`無法播放：${e.name} ${e.message}`);
      });
    },
    [lines, current, pause, generated],
  );

  const playOriginal = useCallback(
    (index: number) => {
      audioRef.current?.pause();
      setSelected(index);
      if (embedBlocked) {
        window.open(youtubeUrl(song.id, spans[index][0]), "_blank", "noopener");
        return;
      }
      playSegment(spans[index][0], spans[index][1] + 0.5);
    },
    [playSegment, spans, embedBlocked, song.id],
  );

  const go = useCallback(
    (delta: number) => pick(Math.min(lines.length - 1, Math.max(0, current + delta))),
    [pick, lines.length, current],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement || e.metaKey || e.ctrlKey) return;
      const actions: Record<string, () => void> = {
        ArrowRight: () => go(1),
        ArrowLeft: () => go(-1),
        n: () => playTeacher("normal"),
        s: () => playTeacher("slow"),
        r: () => playOriginal(current),
      };
      const action = actions[e.key];
      if (action) {
        e.preventDefault();
        action();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [go, playTeacher, playOriginal, current]);

  const line = lines[current];
  const pendingCount = lines.filter(needsAttention).length;
  const staleCount = lines.filter((l) => l.stale).length;

  const nextPending = () => {
    const order = [...lines.keys()].map((k) => (current + 1 + k) % lines.length);
    const next = order.find((i) => needsAttention(lines[i]));
    if (next !== undefined) pick(next);
  };

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="flex flex-col gap-4 lg:sticky lg:top-4 lg:self-start">
        <div className="relative aspect-video w-full overflow-hidden rounded-xl bg-black">
          <div ref={containerRef} />
          {embedBlocked && (
            <EmbedFallback videoId={song.id} code={playerError} start={line ? spans[current][0] : 0} />
          )}
        </div>
        {line && (
          <LineCard
            key={current}
            songId={song.id}
            sameCount={lines.filter((l) => l.text === line.text).length}
            line={line}
            index={current}
            total={lines.length}
            language={language}
            ready={ready}
            onTeacher={playTeacher}
            audioError={audioError}
            generated={generated}
            generating={generating}
            onOriginal={() => playOriginal(current)}
            embedBlocked={embedBlocked}
            onPrev={() => go(-1)}
            onNext={() => go(1)}
          />
        )}
        <p className="text-xs text-stone-500">
          快捷鍵：← → 上下句　N 老師正常速　S 老師慢速　R 原曲這句
        </p>
      </section>

      <section className="min-w-0">
        {staleCount > 0 && <ReannotateBanner songId={song.id} staleCount={staleCount} />}
        {pendingCount > 0 && (
          <div className="mb-2 flex items-center justify-between rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
            <span>還有 {pendingCount} 句待校對</span>
            <button onClick={nextPending} className="rounded px-2 py-0.5 font-medium hover:bg-rose-100 dark:hover:bg-rose-900/40">
              下一句待校對 →
            </button>
          </div>
        )}
        <label className="mb-2 flex items-center gap-2 text-sm text-stone-500">
          <input
            type="checkbox"
            checked={follow && !embedBlocked}
            disabled={embedBlocked}
            onChange={(e) => {
              if (!e.target.checked) setSelected(current);
              setFollow(e.target.checked);
            }}
          />
          {embedBlocked ? "跟著 MV 自動切換歌詞（這支影片無法在頁面內播放）" : "跟著 MV 自動切換歌詞"}
        </label>
        <ol ref={listRef} className="flex flex-col gap-1 lg:max-h-[calc(100vh-8rem)] lg:overflow-y-auto lg:pr-2">
          {lines.map((l, i) => {
            const newSection = l.section && l.section !== lines[i - 1]?.section;
            return (
              <li key={i} data-index={i}>
                {newSection && (
                  <div className="mb-1 mt-3 text-xs font-semibold uppercase tracking-wide text-stone-400">
                    {SECTION_LABELS[l.section!] ?? l.section}
                  </div>
                )}
                <button
                  onClick={() => pick(i)}
                  onDoubleClick={() => playOriginal(i)}
                  className={`w-full rounded-lg px-3 py-2 text-left transition ${
                    i === current
                      ? "bg-amber-100 dark:bg-amber-900/30"
                      : "hover:bg-stone-100 dark:hover:bg-stone-900"
                  } ${i === playing ? "ring-2 ring-amber-400" : ""}`}
                >
                  <div className="flex items-start gap-2">
                    <span className="mt-0.5 w-10 shrink-0 font-mono text-xs text-stone-400">{l.start}</span>
                    <div className="min-w-0">
                      <div className="text-lg leading-snug">
                        {l.text}
                        {needsAttention(l) && (
                          <span title="待校對" className="ml-1 text-xs text-rose-500">
                            ●
                          </span>
                        )}
                        {l.reviewed && (
                          <span title="已校對" className="ml-1 text-xs text-emerald-600">
                            ✓
                          </span>
                        )}
                      </div>
                      {l.romanization && <div className="text-xs text-stone-500">{l.romanization}</div>}
                      {l.translation_zh && <div className="text-sm text-stone-600 dark:text-stone-400">{l.translation_zh}</div>}
                    </div>
                  </div>
                </button>
              </li>
            );
          })}
        </ol>
      </section>
    </div>
  );
}

function ReannotateBanner({ songId, staleCount }: { songId: string; staleCount: number }) {
  const router = useRouter();
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const run = async () => {
    setRunning(true);
    setError(null);
    const res = await fetch(`/api/songs/${songId}/reannotate`, { method: "POST" });
    const body = await res.json().catch(() => ({}));
    setRunning(false);
    if (!res.ok) return setError(body.error ?? `重新分析失敗（${res.status}）`);
    router.refresh();
  };

  return (
    <div className="mb-2 rounded-lg bg-sky-50 px-3 py-2 text-sm text-sky-800 dark:bg-sky-950/40 dark:text-sky-200">
      <div className="flex items-center justify-between gap-2">
        <span>{staleCount} 句歌詞已修改，拼音與單字拆解還是舊的。</span>
        <button
          onClick={run}
          disabled={running}
          className="shrink-0 rounded-full bg-sky-600 px-3 py-1 font-medium text-white hover:bg-sky-700 disabled:opacity-50"
        >
          {running ? "分析中…（約 30 秒）" : "🔄 重新分析整首"}
        </button>
      </div>
      <p className="mt-1 text-xs opacity-80">使用 1 次 Gemini Flash 請求，不佔 TTS 額度；手動修改的翻譯與校對標記會保留。</p>
      {error && <p className="mt-1 text-rose-700 dark:text-rose-300">{error}</p>}
    </div>
  );
}

function LineCard(props: {
  songId: string;
  sameCount: number;
  line: Line;
  index: number;
  total: number;
  language: string;
  ready: boolean;
  onTeacher: (speed: "normal" | "slow") => void;
  audioError: string | null;
  generated: Set<string>;
  generating: string | null;
  onOriginal: () => void;
  embedBlocked: boolean;
  onPrev: () => void;
  onNext: () => void;
}) {
  const { line, index, total, language, ready } = props;
  const tokens = line.tokens ?? [];
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [confirmError, setConfirmError] = useState<string | null>(null);

  const confirmOk = async () => {
    setConfirmError(null);
    const err = await saveEdit(props.songId, index, { reviewed: true, applyToSame: true });
    if (err) setConfirmError(err);
    else router.refresh();
  };
  const label = (speed: string, text: string) =>
    line.audio && props.generating === line.audio[speed] ? "⏳ 生成中…" : text;
  const uncached = line.audio ? Object.values(line.audio).filter((src) => !props.generated.has(src)).length : 0;

  return (
    <article className="rounded-xl border border-stone-200 bg-white p-5 dark:border-stone-800 dark:bg-stone-900">
      <div className="mb-3 flex items-center justify-between text-xs text-stone-500">
        <span>
          第 {index + 1} / {total} 句　{line.start}–{line.end}
          {line.reviewed && <span className="ml-2 text-emerald-600">✓ 已校對</span>}
        </span>
        <div className="flex gap-1">
          <button
            onClick={() => setEditing((v) => !v)}
            className={`rounded px-2 py-1 hover:bg-stone-100 dark:hover:bg-stone-800 ${editing ? "bg-stone-100 dark:bg-stone-800" : ""}`}
          >
            ✏️ 校對
          </button>
          <button onClick={props.onPrev} disabled={index === 0} className="rounded px-2 py-1 hover:bg-stone-100 disabled:opacity-30 dark:hover:bg-stone-800">
            ← 上一句
          </button>
          <button onClick={props.onNext} disabled={index === total - 1} className="rounded px-2 py-1 hover:bg-stone-100 disabled:opacity-30 dark:hover:bg-stone-800">
            下一句 →
          </button>
        </div>
      </div>

      <p className="text-2xl leading-loose">
        {language === "ja" && tokens.length > 0 ? tokens.map((t, i) => <Word key={i} token={t} />) : line.text}
      </p>
      {line.romanization && <p className="mt-1 text-stone-500">{line.romanization}</p>}
      {line.translation_zh && <p className="mt-2 text-lg">{line.translation_zh}</p>}

      {line.stale && (
        <p className="mt-3 rounded-md bg-sky-50 px-3 py-2 text-sm text-sky-800 dark:bg-sky-950/40 dark:text-sky-200">
          這句已修改，拼音與單字拆解要等「重新分析」後才會更新。
        </p>
      )}
      {needsAttention(line) && !line.stale && (
        <div className="mt-3 flex flex-wrap items-center justify-between gap-2 rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
          <span>
            {line.uncertain ? "這句轉錄時聽不太清楚，" : "這句的漢字讀音兩次分析結果不一致，"}建議對照原曲或官方歌詞確認。
          </span>
          <span className="flex gap-1">
            <button onClick={confirmOk} className="rounded px-2 py-0.5 font-medium hover:bg-rose-100 dark:hover:bg-rose-900/40">
              ✓ 沒問題
            </button>
            <button onClick={() => setEditing(true)} className="rounded px-2 py-0.5 font-medium hover:bg-rose-100 dark:hover:bg-rose-900/40">
              ✏️ 修改
            </button>
          </span>
          {confirmError && <span className="w-full">{confirmError}</span>}
        </div>
      )}
      {editing && (
        <ReviewForm
          songId={props.songId}
          index={index}
          line={line}
          language={language}
          sameCount={props.sameCount}
          onDone={() => setEditing(false)}
        />
      )}

      <div className="mt-4 flex flex-wrap gap-2">
        <button
          onClick={() => props.onTeacher("normal")}
          disabled={!line.audio}
          className="rounded-full bg-amber-500 px-4 py-2 text-sm font-medium text-white hover:bg-amber-600 disabled:opacity-40"
        >
          {label("normal", "🔊 老師念")}
        </button>
        <button
          onClick={() => props.onTeacher("slow")}
          disabled={!line.audio}
          className="rounded-full bg-amber-100 px-4 py-2 text-sm font-medium text-amber-900 hover:bg-amber-200 disabled:opacity-40 dark:bg-amber-900/40 dark:text-amber-100"
        >
          {label("slow", "🐢 慢速")}
        </button>
        <button
          onClick={props.onOriginal}
          disabled={!ready && !props.embedBlocked}
          className="rounded-full border border-stone-300 px-4 py-2 text-sm font-medium hover:bg-stone-100 disabled:opacity-40 dark:border-stone-700 dark:hover:bg-stone-800"
        >
          {props.embedBlocked ? "🎵 在 YouTube 聽這句 ↗" : "🎵 原曲這句"}
        </button>
      </div>
      {props.audioError && (
        <p className="mt-2 rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
          {props.audioError}
        </p>
      )}
      {!line.audio && (
        <p className="mt-3 rounded-md bg-sky-50 px-3 py-2 text-sm text-sky-800 dark:bg-sky-950/40 dark:text-sky-200">
          這個語言還沒有老師聲音設定（config/teachers.json）。
        </p>
      )}
      {uncached > 0 && (
        <p className="mt-2 text-xs text-stone-500">
          第一次播放會即時生成示範音（約 5 秒，每段用掉 1 次 TTS 額度），之後就直接播放。
        </p>
      )}

      {tokens.length > 0 && (
        <div className="mt-5">
          <h3 className="mb-2 text-sm font-semibold text-stone-500">單字拆解</h3>
          <ul className="flex flex-wrap gap-2">
            {tokens.map((t, i) => (
              <li key={i} className="rounded-lg border border-stone-200 px-2.5 py-1.5 dark:border-stone-700">
                <div className="font-medium">{t.surface}</div>
                {t.reading && t.reading !== t.surface && <div className="text-xs text-stone-500">{t.reading}</div>}
                <div className="text-xs">
                  <span className="text-stone-400">{POS_LABELS[t.pos] ?? t.pos}</span> {t.meaning_zh}
                </div>
              </li>
            ))}
          </ul>
        </div>
      )}

      {line.grammar_note && (
        <div className="mt-4">
          <h3 className="mb-1 text-sm font-semibold text-stone-500">文法重點</h3>
          <p className="text-sm leading-relaxed">{line.grammar_note}</p>
        </div>
      )}
      {line.pronunciation_tip && (
        <div className="mt-4">
          <h3 className="mb-1 text-sm font-semibold text-stone-500">發音提示</h3>
          <p className="text-sm leading-relaxed">{line.pronunciation_tip}</p>
        </div>
      )}
    </article>
  );
}
