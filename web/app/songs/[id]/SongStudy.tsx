"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { POS_LABELS, SECTION_LABELS } from "@/lib/labels";
import type { Line, Song, Token } from "@/lib/songs";
import { useYouTubePlayer } from "@/lib/useYouTubePlayer";

const KANJI = /[一-鿿々]/;

function toSeconds(timestamp: string): number {
  const [m, s] = timestamp.split(":").map(Number);
  return m * 60 + s;
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
  const { containerRef, ready, time, playSegment, pause } = useYouTubePlayer(song.id);
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
      playSegment(spans[index][0], spans[index][1] + 0.5);
    },
    [playSegment, spans],
  );

  const go = useCallback(
    (delta: number) => pick(Math.min(lines.length - 1, Math.max(0, current + delta))),
    [pick, lines.length, current],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.metaKey || e.ctrlKey) return;
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

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]">
      <section className="flex flex-col gap-4 lg:sticky lg:top-4 lg:self-start">
        <div className="aspect-video w-full overflow-hidden rounded-xl bg-black">
          <div ref={containerRef} />
        </div>
        {line && (
          <LineCard
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
            onPrev={() => go(-1)}
            onNext={() => go(1)}
          />
        )}
        <p className="text-xs text-stone-500">
          快捷鍵：← → 上下句　N 老師正常速　S 老師慢速　R 原曲這句
        </p>
      </section>

      <section className="min-w-0">
        <label className="mb-2 flex items-center gap-2 text-sm text-stone-500">
          <input
            type="checkbox"
            checked={follow}
            onChange={(e) => {
              if (!e.target.checked) setSelected(current);
              setFollow(e.target.checked);
            }}
          />
          跟著 MV 自動切換歌詞
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
                        {l.needs_review && (
                          <span title="讀音可能有誤，請校對" className="ml-1 text-xs text-rose-500">
                            ●
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

function LineCard(props: {
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
  onPrev: () => void;
  onNext: () => void;
}) {
  const { line, index, total, language, ready } = props;
  const tokens = line.tokens ?? [];
  const label = (speed: string, text: string) =>
    line.audio && props.generating === line.audio[speed] ? "⏳ 生成中…" : text;
  const uncached = line.audio ? Object.values(line.audio).filter((src) => !props.generated.has(src)).length : 0;

  return (
    <article className="rounded-xl border border-stone-200 bg-white p-5 dark:border-stone-800 dark:bg-stone-900">
      <div className="mb-3 flex items-center justify-between text-xs text-stone-500">
        <span>
          第 {index + 1} / {total} 句　{line.start}–{line.end}
        </span>
        <div className="flex gap-1">
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

      {(line.needs_review || line.uncertain) && (
        <p className="mt-3 rounded-md bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
          {line.uncertain ? "這句轉錄時聽不太清楚，" : "這句的漢字讀音兩次分析結果不一致，"}建議對照原曲或官方歌詞確認。
        </p>
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
          disabled={!ready}
          className="rounded-full border border-stone-300 px-4 py-2 text-sm font-medium hover:bg-stone-100 disabled:opacity-40 dark:border-stone-700 dark:hover:bg-stone-800"
        >
          🎵 原曲這句
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
