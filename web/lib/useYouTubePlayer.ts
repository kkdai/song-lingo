"use client";

import { useCallback, useEffect, useRef, useState } from "react";

type YTPlayer = {
  getCurrentTime(): number;
  seekTo(seconds: number, allowSeekAhead: boolean): void;
  playVideo(): void;
  pauseVideo(): void;
  destroy(): void;
};

declare global {
  interface Window {
    YT?: { Player: new (el: HTMLElement, opts: object) => YTPlayer };
    onYouTubeIframeAPIReady?: () => void;
  }
}

let apiPromise: Promise<void> | null = null;

function loadApi(): Promise<void> {
  if (window.YT?.Player) return Promise.resolve();
  apiPromise ??= new Promise((resolve) => {
    window.onYouTubeIframeAPIReady = () => resolve();
    const script = document.createElement("script");
    script.src = "https://www.youtube.com/iframe_api";
    document.head.appendChild(script);
  });
  return apiPromise;
}

/** Embeds a YouTube player and reports its playback time (polled, in seconds). */
export function useYouTubePlayer(videoId: string) {
  const containerRef = useRef<HTMLDivElement>(null);
  const playerRef = useRef<YTPlayer | null>(null);
  const stopAtRef = useRef<number | null>(null);
  const [ready, setReady] = useState(false);
  const [time, setTime] = useState(0);

  useEffect(() => {
    let cancelled = false;
    loadApi().then(() => {
      if (cancelled || !containerRef.current || !window.YT) return;
      playerRef.current = new window.YT.Player(containerRef.current, {
        videoId,
        width: "100%",
        height: "100%",
        playerVars: { rel: 0, playsinline: 1, modestbranding: 1 },
        events: { onReady: () => !cancelled && setReady(true) },
      });
    });
    return () => {
      cancelled = true;
      playerRef.current?.destroy();
      playerRef.current = null;
      setReady(false);
    };
  }, [videoId]);

  useEffect(() => {
    if (!ready) return;
    const timer = setInterval(() => {
      const player = playerRef.current;
      if (!player) return;
      const t = player.getCurrentTime();
      setTime(t);
      if (stopAtRef.current !== null && t >= stopAtRef.current) {
        player.pauseVideo();
        stopAtRef.current = null;
      }
    }, 200);
    return () => clearInterval(timer);
  }, [ready]);

  /** Play from `start`; if `end` is given, pause automatically when reaching it. */
  const playSegment = useCallback((start: number, end?: number) => {
    const player = playerRef.current;
    if (!player) return;
    stopAtRef.current = end ?? null;
    player.seekTo(start, true);
    player.playVideo();
  }, []);

  const pause = useCallback(() => {
    stopAtRef.current = null;
    playerRef.current?.pauseVideo();
  }, []);

  return { containerRef, ready, time, playSegment, pause };
}
