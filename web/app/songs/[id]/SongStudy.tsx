"use client";

import { useRouter } from "next/navigation";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { POS_LABELS, SECTION_LABELS } from "@/lib/labels";
import type { Progress } from "@/lib/progress";
import { type LineProgress, type ShadowAttempt, withAttempt } from "@/lib/progressShared";
import type { Line, Song, Token } from "@/lib/songs";
import { type Recording, useRecorder } from "@/lib/useRecorder";
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

export type ShadowResult = {
  transcript: string;
  score: number;
  words: { surface: string; status: "ok" | "wrong" | "missing" }[];
  /** The attempt as recorded in the song's progress. */
  attempt: ShadowAttempt;
  /** False when scoring worked but the progress store couldn't record it. */
  saved: boolean;
};

/** One shadowing attempt on a line: recording → scoring → result (or error). */
type Shadow = {
  index: number;
  status: "starting" | "recording" | "scoring" | "done" | "error";
  result?: ShadowResult;
  error?: string;
  audioUrl?: string;
};

function micError(e: unknown): string {
  const name = e instanceof DOMException ? e.name : "";
  if (name === "NotAllowedError") return "麥克風權限被拒絕，請在瀏覽器設定裡允許這個網站使用麥克風。";
  if (name === "NotFoundError") return "找不到麥克風。";
  return e instanceof Error ? e.message : "無法開始錄音。";
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

/** Save a progress change; resolves to an error message, or null on success. */
async function saveProgress(songId: string, change: { lastLine?: number; learned?: { index: number; value: boolean } }) {
  const res = await fetch(`/api/songs/${songId}/progress`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(change),
    // Lets the last position save finish while the page is being left.
    keepalive: true,
  }).catch(() => null);
  if (res?.ok) return null;
  const body = await res?.json().catch(() => null);
  return body?.error ?? "進度儲存失敗。";
}

export default function SongStudy({ song, initialProgress }: { song: Song; initialProgress: Progress }) {
  const { lines, language } = song;
  const { containerRef, ready, time, error: playerError, playSegment, pause } = useYouTubePlayer(song.id);
  const embedBlocked = playerError !== null;
  // Resume where the learner left off.
  const [selected, setSelected] = useState(() => Math.min(initialProgress.lastLine, Math.max(lines.length - 1, 0)));
  // Learned flags and shadowing history, keyed by line text.
  const [progress, setProgress] = useState<Record<string, LineProgress>>(initialProgress.lines);
  const [progressError, setProgressError] = useState<string | null>(null);
  const [follow, setFollow] = useState(true);
  const [audioError, setAudioError] = useState<string | null>(null);
  // Clip URLs known to exist on the server; others are generated on first play.
  const [generated, setGenerated] = useState(
    () => new Set(lines.flatMap((l) => (l.audioCached ?? []).map((speed) => l.audio![speed]))),
  );
  const [generating, setGenerating] = useState<string | null>(null);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const [sheetOpen, setSheetOpen] = useState(false);
  const [shadow, setShadow] = useState<Shadow | null>(null);
  const recordingIndex = useRef(0);
  const takeUrl = useRef<string | null>(null);

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

  const submitTake = useCallback(
    async (index: number, take: Recording | null) => {
      if (!take || take.ms < 500) {
        setShadow({ index, status: "error", error: "錄音太短了，念完整句再按停止。" });
        return;
      }
      if (takeUrl.current) URL.revokeObjectURL(takeUrl.current);
      const audioUrl = (takeUrl.current = URL.createObjectURL(take.blob));
      setShadow({ index, status: "scoring", audioUrl });
      const form = new FormData();
      form.append("audio", take.blob, "take");
      const res = await fetch(`/api/songs/${song.id}/lines/${index}/shadow`, { method: "POST", body: form }).catch(() => null);
      const body = await res?.json().catch(() => null);
      if (res?.ok) {
        setShadow({ index, status: "done", result: body, audioUrl });
        const key = lines[index].progressKey!;
        setProgress((p) => ({ ...p, [key]: withAttempt(p[key], body.attempt) }));
      }
      else setShadow({ index, status: "error", error: body?.error ?? "評分失敗，請再試一次。", audioUrl });
    },
    [song.id, lines],
  );

  const recorder = useRecorder((take) => submitTake(recordingIndex.current, take));

  const toggleShadow = useCallback(async () => {
    if (recorder.recording) {
      submitTake(recordingIndex.current, await recorder.stop());
      return;
    }
    pause();
    audioRef.current?.pause();
    recordingIndex.current = current;
    // Only show "recording" once the mic is actually live: until then (e.g. the permission prompt
    // is open) the button is disabled, so a second tap can't start a second getUserMedia.
    setShadow({ index: current, status: "starting" });
    try {
      await recorder.start();
      setShadow((s) => (s?.index === recordingIndex.current && s.status === "starting" ? { ...s, status: "recording" } : s));
    } catch (e) {
      setShadow({ index: current, status: "error", error: micError(e) });
    }
  }, [recorder, submitTake, pause, current]);

  // Moving to another line mid-recording discards that take instead of scoring it.
  useEffect(() => {
    if (recorder.recording && recordingIndex.current !== current) recorder.stop().then(() => setShadow(null));
  }, [current, recorder]);

  useEffect(() => () => void (takeUrl.current && URL.revokeObjectURL(takeUrl.current)), []);

  // Remember the current line, once the learner has stayed on it for a moment.
  const savedLine = useRef(selected);
  useEffect(() => {
    if (current === savedLine.current) return;
    const timer = setTimeout(() => {
      savedLine.current = current;
      saveProgress(song.id, { lastLine: current });
    }, 1500);
    return () => clearTimeout(timer);
  }, [current, song.id]);

  const isLearned = useCallback((i: number) => Boolean(progress[lines[i].progressKey!]?.learned), [progress, lines]);

  const toggleLearned = useCallback(
    async (index: number) => {
      const key = lines[index].progressKey!;
      const value = !progress[key]?.learned;
      const set = (learned: boolean) => setProgress((p) => ({ ...p, [key]: { ...p[key], learned } }));
      set(value);
      setProgressError(null);
      const error = await saveProgress(song.id, { learned: { index, value } });
      if (error) {
        set(!value);
        setProgressError(error);
      }
    },
    [lines, progress, song.id],
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
        l: () => toggleLearned(current),
      };
      const action = actions[e.key];
      if (action) {
        e.preventDefault();
        action();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [go, playTeacher, playOriginal, toggleLearned, current]);

  const line = lines[current];
  const pendingCount = lines.filter(needsAttention).length;
  const staleCount = lines.filter((l) => l.stale).length;

  const learnedCount = lines.filter((_, i) => isLearned(i)).length;
  const nextUnlearned = () => {
    const order = [...lines.keys()].map((k) => (current + 1 + k) % lines.length);
    const next = order.find((i) => !isLearned(i));
    if (next !== undefined) pick(next);
  };

  const nextPending = () => {
    const order = [...lines.keys()].map((k) => (current + 1 + k) % lines.length);
    const next = order.find((i) => needsAttention(lines[i]));
    if (next !== undefined) pick(next);
  };

  const listProps = { lines, progress, current, playing, onPick: pick, onPlayOriginal: playOriginal };
  const listHeader = (
    <ListHeader
      songId={song.id}
      staleCount={staleCount}
      pendingCount={pendingCount}
      onNextPending={nextPending}
      learnedCount={learnedCount}
      total={lines.length}
      onNextUnlearned={nextUnlearned}
      follow={follow && !embedBlocked}
      embedBlocked={embedBlocked}
      onFollowChange={(on) => {
        if (!on) setSelected(current);
        setFollow(on);
      }}
    />
  );

  return (
    // Mobile: MV pinned on top, the study card below it, controls in a bottom bar and the lyric list
    // in a sheet. Desktop (lg): MV + card on the left, the full lyric list on the right.
    <div className="grid gap-4 pb-36 lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)] lg:gap-6 lg:pb-0">
      <section className="flex flex-col gap-4 lg:sticky lg:top-4 lg:self-start">
        <div className="sticky top-0 z-20 -mx-4 bg-background pt-[env(safe-area-inset-top)] lg:static lg:mx-0 lg:pt-0">
          <div className="relative aspect-video w-full overflow-hidden bg-black lg:rounded-xl">
            <div ref={containerRef} />
            {embedBlocked && (
              <EmbedFallback videoId={song.id} code={playerError} start={line ? spans[current][0] : 0} />
            )}
          </div>
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
            shadow={shadow?.index === current ? shadow : null}
            onShadow={toggleShadow}
            learned={isLearned(current)}
            attempts={progress[line.progressKey!]?.shadow ?? []}
            onToggleLearned={() => toggleLearned(current)}
            progressError={progressError}
          />
        )}
        <p className="hidden text-xs text-stone-500 lg:block">
          快捷鍵：← → 上下句　N 老師正常速　S 老師慢速　R 原曲這句　L 標記已學會
        </p>
      </section>

      <section className="hidden min-w-0 lg:block">
        {listHeader}
        <LyricList {...listProps} className="lg:max-h-[calc(100vh-8rem)] lg:overflow-y-auto lg:pr-2" />
      </section>

      {line && (
        <MobileControls
          line={line}
          index={current}
          total={lines.length}
          ready={ready}
          embedBlocked={embedBlocked}
          generating={generating}
          pendingCount={pendingCount}
          onTeacher={playTeacher}
          onOriginal={() => playOriginal(current)}
          onPrev={() => go(-1)}
          onNext={() => go(1)}
          onOpenList={() => setSheetOpen(true)}
          shadowStatus={shadow?.index === current ? shadow.status : null}
          onShadow={toggleShadow}
        />
      )}
      {sheetOpen && (
        <LyricSheet onClose={() => setSheetOpen(false)}>
          {listHeader}
          <LyricList
            {...listProps}
            onPick={(i) => {
              pick(i);
              setSheetOpen(false);
            }}
          />
        </LyricSheet>
      )}
    </div>
  );
}

/** Stale/pending banners and the follow-the-MV toggle shown above the lyric list. */
function ListHeader(props: {
  songId: string;
  staleCount: number;
  pendingCount: number;
  onNextPending: () => void;
  learnedCount: number;
  total: number;
  onNextUnlearned: () => void;
  follow: boolean;
  embedBlocked: boolean;
  onFollowChange: (on: boolean) => void;
}) {
  const allLearned = props.learnedCount === props.total;
  return (
    <>
      <div className="mb-2 rounded-lg bg-emerald-50 px-3 py-2 text-sm text-emerald-800 dark:bg-emerald-950/40 dark:text-emerald-200">
        <div className="flex items-center justify-between gap-2">
          <span>{allLearned ? "🎉 整首都學會了！" : `已學會 ${props.learnedCount} / ${props.total} 句`}</span>
          {!allLearned && (
            <button onClick={props.onNextUnlearned} className="rounded px-2 py-0.5 font-medium hover:bg-emerald-100 dark:hover:bg-emerald-900/40">
              下一句未學會 →
            </button>
          )}
        </div>
        <div className="mt-1.5 h-1.5 overflow-hidden rounded-full bg-emerald-100 dark:bg-emerald-900/40">
          <div className="h-full rounded-full bg-emerald-500 transition-all" style={{ width: `${(100 * props.learnedCount) / Math.max(props.total, 1)}%` }} />
        </div>
      </div>
      {props.staleCount > 0 && <ReannotateBanner songId={props.songId} staleCount={props.staleCount} />}
      {props.pendingCount > 0 && (
        <div className="mb-2 flex items-center justify-between rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700 dark:bg-rose-950/40 dark:text-rose-300">
          <span>還有 {props.pendingCount} 句待校對</span>
          <button onClick={props.onNextPending} className="rounded px-2 py-0.5 font-medium hover:bg-rose-100 dark:hover:bg-rose-900/40">
            下一句待校對 →
          </button>
        </div>
      )}
      <label className="mb-2 flex items-center gap-2 text-sm text-stone-500">
        <input
          type="checkbox"
          checked={props.follow}
          disabled={props.embedBlocked}
          onChange={(e) => props.onFollowChange(e.target.checked)}
        />
        {props.embedBlocked ? "跟著 MV 自動切換歌詞（這支影片無法在頁面內播放）" : "跟著 MV 自動切換歌詞"}
      </label>
    </>
  );
}

function LyricList(props: {
  lines: Line[];
  progress: Record<string, LineProgress>;
  current: number;
  playing: number | null;
  onPick: (index: number) => void;
  onPlayOriginal: (index: number) => void;
  className?: string;
}) {
  const { lines, current, playing } = props;
  const listRef = useRef<HTMLOListElement>(null);

  useEffect(() => {
    listRef.current
      ?.querySelector(`[data-index="${current}"]`)
      ?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }, [current]);

  return (
    <ol ref={listRef} className={`flex flex-col gap-1 ${props.className ?? ""}`}>
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
              onClick={() => props.onPick(i)}
              onDoubleClick={() => props.onPlayOriginal(i)}
              className={`w-full rounded-lg px-3 py-2 text-left transition ${
                i === current ? "bg-amber-100 dark:bg-amber-900/30" : "hover:bg-stone-100 dark:hover:bg-stone-900"
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
                    {props.progress[l.progressKey!]?.learned && (
                      <span title="已學會" className="ml-1 text-xs text-amber-500">
                        ★
                      </span>
                    )}
                    <BestScore attempts={props.progress[l.progressKey!]?.shadow} />
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
  );
}

/** Thumb-reachable playback and navigation bar pinned to the bottom of the screen on phones. */
function MobileControls(props: {
  line: Line;
  index: number;
  total: number;
  ready: boolean;
  embedBlocked: boolean;
  generating: string | null;
  pendingCount: number;
  onTeacher: (speed: "normal" | "slow") => void;
  onOriginal: () => void;
  onPrev: () => void;
  onNext: () => void;
  onOpenList: () => void;
  shadowStatus: Shadow["status"] | null;
  onShadow: () => void;
}) {
  const { line, index, total } = props;
  const busy = (speed: string) => Boolean(line.audio && props.generating === line.audio[speed]);
  const button =
    "flex h-12 min-w-12 flex-1 flex-col items-center justify-center rounded-xl text-xs font-medium active:scale-95 disabled:opacity-40";

  return (
    <nav className="fixed inset-x-0 bottom-0 z-30 border-t border-stone-200 bg-white/95 px-3 pb-[max(0.5rem,env(safe-area-inset-bottom))] pt-2 backdrop-blur lg:hidden dark:border-stone-800 dark:bg-stone-950/95">
      <div className="mb-2 flex items-center justify-between text-xs text-stone-500">
        <span className="font-mono">
          {index + 1} / {total}　{line.start}
        </span>
        <ShadowButton status={props.shadowStatus} onClick={props.onShadow} className="h-11 px-4 text-sm" />
        <button onClick={props.onOpenList} className="h-11 rounded-full bg-stone-100 px-4 text-sm font-medium text-stone-700 dark:bg-stone-800 dark:text-stone-200">
          📜 歌詞{props.pendingCount > 0 && <span className="ml-1 text-rose-500">●</span>}
        </button>
      </div>
      <div className="flex gap-2">
        <button onClick={props.onPrev} disabled={index === 0} aria-label="上一句" className={`${button} bg-stone-100 text-lg dark:bg-stone-800`}>
          ‹
        </button>
        <button
          onClick={() => props.onTeacher("normal")}
          disabled={!line.audio}
          className={`${button} flex-[1.4] bg-amber-500 text-white`}
        >
          <span className="text-lg">{busy("normal") ? "⏳" : "🔊"}</span>
          {busy("normal") ? "生成中" : "老師念"}
        </button>
        <button
          onClick={() => props.onTeacher("slow")}
          disabled={!line.audio}
          className={`${button} flex-[1.4] bg-amber-100 text-amber-900 dark:bg-amber-900/40 dark:text-amber-100`}
        >
          <span className="text-lg">{busy("slow") ? "⏳" : "🐢"}</span>
          {busy("slow") ? "生成中" : "慢速"}
        </button>
        <button
          onClick={props.onOriginal}
          disabled={!props.ready && !props.embedBlocked}
          className={`${button} flex-[1.4] border border-stone-300 dark:border-stone-700`}
        >
          <span className="text-lg">🎵</span>
          {props.embedBlocked ? "YouTube ↗" : "原曲"}
        </button>
        <button onClick={props.onNext} disabled={index === total - 1} aria-label="下一句" className={`${button} bg-stone-100 text-lg dark:bg-stone-800`}>
          ›
        </button>
      </div>
    </nav>
  );
}

function ShadowButton({ status, onClick, className }: { status: Shadow["status"] | null; onClick: () => void; className: string }) {
  const recording = status === "recording";
  return (
    <button
      onClick={onClick}
      disabled={status === "scoring" || status === "starting"}
      className={`rounded-full font-medium disabled:opacity-50 ${className} ${
        recording ? "animate-pulse bg-rose-600 text-white" : "bg-stone-800 text-white dark:bg-stone-200 dark:text-stone-900"
      }`}
    >
      {recording ? "■ 停止並評分" : status === "scoring" ? "評分中…" : status === "starting" ? "準備麥克風…" : "🎙 跟讀"}
    </button>
  );
}

const WORD_STYLES: Record<ShadowResult["words"][number]["status"], string> = {
  ok: "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/40 dark:text-emerald-100",
  wrong: "bg-rose-100 text-rose-900 dark:bg-rose-900/40 dark:text-rose-100",
  missing: "bg-stone-100 text-stone-400 line-through dark:bg-stone-800",
};

/** Progress and result of a shadowing attempt, shown on the study card. */
function ShadowPanel({ shadow }: { shadow: Shadow }) {
  const [playing, setPlaying] = useState(false);
  const playTake = () => {
    if (!shadow.audioUrl) return;
    const audio = new Audio(shadow.audioUrl);
    audio.onended = () => setPlaying(false);
    setPlaying(true);
    audio.play().catch(() => setPlaying(false));
  };
  const result = shadow.result;

  return (
    <div className="mt-4 rounded-lg border border-stone-200 p-3 text-sm dark:border-stone-700">
      {shadow.status === "starting" && <p className="text-stone-500">正在開啟麥克風⋯⋯第一次使用時，請允許瀏覽器使用麥克風。</p>}
      {shadow.status === "recording" && <p className="text-rose-600">🔴 錄音中⋯⋯念完這句後再按「停止並評分」。</p>}
      {shadow.status === "scoring" && <p className="text-stone-500">評分中⋯⋯大約 10 秒。</p>}
      {shadow.status === "error" && <p className="text-rose-700 dark:text-rose-300">{shadow.error}</p>}
      {shadow.status === "done" && result && (
        <>
          <div className="flex items-baseline justify-between gap-2">
            <span className="font-semibold text-stone-500">跟讀結果</span>
            <span className={`text-2xl font-bold ${scoreColor(result.score)}`}>
              {result.score} 分
            </span>
          </div>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {result.words.map((w, i) => (
              <span key={i} className={`rounded px-1.5 py-0.5 text-base ${WORD_STYLES[w.status]}`}>
                {w.surface}
              </span>
            ))}
          </div>
          <p className="mt-2 text-stone-500">
            你念的是：<span className="text-stone-800 dark:text-stone-200">{result.transcript}</span>
          </p>
          {!result.saved && (
            <p className="mt-2 text-amber-700 dark:text-amber-300">這次的分數沒有存進學習紀錄（儲存失敗），不影響評分結果。</p>
          )}
          <p className="mt-1 text-xs text-stone-400">綠色：念對　紅色：念成別的詞　灰色：沒念到。只檢查聽不聽得出是哪個詞，音調與長短音不在評分範圍。</p>
        </>
      )}
      {shadow.audioUrl && shadow.status !== "scoring" && (
        <button onClick={playTake} disabled={playing} className="mt-2 rounded-full border border-stone-300 px-3 py-1 text-xs disabled:opacity-50 dark:border-stone-600">
          {playing ? "播放中…" : "▶ 聽我的錄音"}
        </button>
      )}
    </div>
  );
}

function scoreColor(score: number): string {
  return score >= 80 ? "text-emerald-600" : score >= 50 ? "text-amber-600" : "text-rose-600";
}

/** Best shadowing score of a line, shown next to it in the lyric list. */
function BestScore({ attempts }: { attempts?: ShadowAttempt[] }) {
  if (!attempts?.length) return null;
  const best = Math.max(...attempts.map((a) => a.score));
  return (
    <span title={`跟讀 ${attempts.length} 次，最高 ${best} 分`} className={`ml-1.5 font-mono text-xs ${scoreColor(best)}`}>
      {best}
    </span>
  );
}

/** Past shadowing scores of the current line, oldest first, and the words most often missed. */
function ShadowHistory({ attempts }: { attempts: ShadowAttempt[] }) {
  const best = Math.max(...attempts.map((a) => a.score));
  const missed = new Map<string, number>();
  for (const word of attempts.slice(-5).flatMap((a) => a.missed)) missed.set(word, (missed.get(word) ?? 0) + 1);
  const often = [...missed].filter(([, n]) => n >= 2).sort((a, b) => b[1] - a[1]).slice(0, 5);

  return (
    <div className="mt-3 text-xs text-stone-500">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <span>
          跟讀 {attempts.length} 次・最高 <span className={`font-semibold ${scoreColor(best)}`}>{best}</span> 分
        </span>
        <span className="flex gap-1 font-mono" aria-label="最近的分數">
          {attempts.slice(-8).map((a, i) => (
            // Server and browser format dates slightly differently; the browser's wins.
            <span key={i} title={new Date(a.at).toLocaleString("zh-TW")} suppressHydrationWarning className={scoreColor(a.score)}>
              {a.score}
            </span>
          ))}
        </span>
      </div>
      {often.length > 0 && <div className="mt-1">最近常沒念好：{often.map(([w]) => w).join("、")}</div>}
    </div>
  );
}

/** Bottom sheet holding the full lyric list on phones. */
function LyricSheet({ onClose, children }: { onClose: () => void; children: React.ReactNode }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    // Keep the page behind the sheet from scrolling.
    const overflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      window.removeEventListener("keydown", onKey);
      document.body.style.overflow = overflow;
    };
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-40 lg:hidden" role="dialog" aria-modal="true" aria-label="歌詞列表">
      <button className="absolute inset-0 bg-black/40" aria-label="關閉" onClick={onClose} />
      <div className="absolute inset-x-0 bottom-0 flex max-h-[80vh] flex-col rounded-t-2xl bg-white pb-[env(safe-area-inset-bottom)] dark:bg-stone-900">
        <div className="flex items-center justify-between border-b border-stone-200 px-4 py-3 dark:border-stone-800">
          <span className="font-semibold">歌詞</span>
          <button onClick={onClose} className="rounded-full px-3 py-1 text-sm text-stone-500 hover:bg-stone-100 dark:hover:bg-stone-800">
            關閉
          </button>
        </div>
        <div className="overflow-y-auto overscroll-contain px-3 py-2">{children}</div>
      </div>
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
  shadow: Shadow | null;
  onShadow: () => void;
  learned: boolean;
  attempts: ShadowAttempt[];
  onToggleLearned: () => void;
  progressError: string | null;
}) {
  const { line, index, total, language, ready } = props;
  const tokens = line.tokens ?? [];
  const router = useRouter();
  const [editing, setEditing] = useState(false);
  const [confirmError, setConfirmError] = useState<string | null>(null);
  const touchStart = useRef<{ x: number; y: number } | null>(null);

  // Horizontal swipe on the card switches lines (phones). Ignored while the review form is open.
  const onTouchStart = (e: React.TouchEvent) => {
    const t = e.touches[0];
    touchStart.current = editing ? null : { x: t.clientX, y: t.clientY };
  };
  const onTouchEnd = (e: React.TouchEvent) => {
    const start = touchStart.current;
    touchStart.current = null;
    if (!start) return;
    const t = e.changedTouches[0];
    const dx = t.clientX - start.x;
    const dy = t.clientY - start.y;
    if (Math.abs(dx) < 60 || Math.abs(dx) < Math.abs(dy) * 1.5) return;
    if (dx < 0 && index < total - 1) props.onNext();
    if (dx > 0 && index > 0) props.onPrev();
  };

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
    <article
      onTouchStart={onTouchStart}
      onTouchEnd={onTouchEnd}
      className="rounded-xl border border-stone-200 bg-white p-4 dark:border-stone-800 dark:bg-stone-900 lg:p-5"
    >
      <div className="mb-3 flex items-center justify-between text-xs text-stone-500">
        <span>
          第 {index + 1} / {total} 句　{line.start}–{line.end}
          {line.reviewed && <span className="ml-2 text-emerald-600">✓ 已校對</span>}
        </span>
        <div className="flex gap-1">
          <button
            onClick={props.onToggleLearned}
            aria-pressed={props.learned}
            className={`rounded px-2 py-1 ${
              props.learned
                ? "bg-amber-100 font-medium text-amber-800 dark:bg-amber-900/40 dark:text-amber-200"
                : "hover:bg-stone-100 dark:hover:bg-stone-800"
            }`}
          >
            {props.learned ? "★ 已學會" : "☆ 學會了"}
          </button>
          <button
            onClick={() => setEditing((v) => !v)}
            className={`rounded px-2 py-1 hover:bg-stone-100 dark:hover:bg-stone-800 ${editing ? "bg-stone-100 dark:bg-stone-800" : ""}`}
          >
            ✏️ 校對
          </button>
          <button onClick={props.onPrev} disabled={index === 0} className="hidden rounded px-2 py-1 hover:bg-stone-100 disabled:opacity-30 dark:hover:bg-stone-800 lg:inline-block">
            ← 上一句
          </button>
          <button onClick={props.onNext} disabled={index === total - 1} className="hidden rounded px-2 py-1 hover:bg-stone-100 disabled:opacity-30 dark:hover:bg-stone-800 lg:inline-block">
            下一句 →
          </button>
        </div>
      </div>

      <p className="text-[1.75rem] leading-loose lg:text-2xl">
        {language === "ja" && tokens.length > 0 ? tokens.map((t, i) => <Word key={i} token={t} />) : line.text}
      </p>
      {line.romanization && <p className="mt-1 text-stone-500">{line.romanization}</p>}
      {line.translation_zh && <p className="mt-2 text-xl lg:text-lg">{line.translation_zh}</p>}
      {props.shadow && <ShadowPanel shadow={props.shadow} />}
      {props.attempts.length > 0 && <ShadowHistory attempts={props.attempts} />}
      {props.progressError && <p className="mt-2 text-sm text-rose-700 dark:text-rose-300">{props.progressError}</p>}

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

      <div className="mt-4 hidden flex-wrap gap-2 lg:flex">
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
        <ShadowButton status={props.shadow?.status ?? null} onClick={props.onShadow} className="px-4 py-2 text-sm" />
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
