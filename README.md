# Song Lingo

**English** | [繁體中文](README_tw.md)

Learn a language through the songs you love. Paste a YouTube MV link, and Song Lingo transcribes the lyrics, adds romanization, translation and grammar notes, and gives you a teacher who reads every line aloud — at normal speed or slowly — so you can study the song one line at a time.

> Lyrics are copyrighted. Song Lingo is a **personal study tool**: transcripts stay on your machine (or your private bucket), are git-ignored, and should never be committed or redistributed.

## What it does

- **Add a song from a YouTube link** — Gemini watches the MV (preferring on-screen lyric subtitles when present) and transcribes every sung line with timestamps.
- **Line-by-line study cards** — furigana over kanji, romanization, Traditional Chinese translation, a word-by-word breakdown (reading, part of speech, meaning), one grammar point and one pronunciation tip per line.
- **A teacher who reads each line** — a voice designed from a text description with Gemini TTS, in a normal and a slow, clearly articulated version.
- **Study alongside the MV** — the lyric list follows the video; replay just the current line of the original song.
- **Review and correct** — lines likely to be wrong are flagged; fix the text, reading or translation in place, then re-analyze the song.
- **Languages** — Japanese and Korean (with romanization) and English (with vocabulary and linking notes).

## Built with

| Part | Technology |
|---|---|
| Lyric transcription | `gemini-3.8-flash` reading a YouTube URL directly, with structured JSON output |
| Translation, word breakdown, grammar | `gemini-3.8-flash` |
| Teacher voice | `gemini-3.8-flash-tts` — voice design (a voice created from a text prompt) and per-line speaking style |
| Romanization | Gemini for word segmentation and part of speech; [pykakasi](https://github.com/miurahr/pykakasi) for kana → Hepburn, with particle fixes (は→wa, へ→e, を→o); Revised Romanization for Korean |
| Pipeline scripts | Python 3.12, [uv](https://docs.astral.sh/uv/), [google-genai](https://github.com/googleapis/python-genai) |
| Web app | [Next.js](https://nextjs.org/) 16 (App Router), React 19, Tailwind CSS 4 |
| Video | YouTube IFrame Player API |

## How it works

```
YouTube URL
  → transcribe.py   lyrics + timestamps (+ hiragana readings for Japanese)
  → annotate.py     romanization, zh-TW translation, word breakdown, grammar & pronunciation notes
  → web app         study line by line next to the MV
                    teacher audio is generated the first time a line is played, then cached
```

A few design choices worth knowing:

- **Two independent readings as a cross-check.** Transcription and annotation each produce a reading for Japanese lines; when they disagree, the line is flagged for review — no extra API call needed.
- **Audio on demand.** Each clip is synthesized the first time it's played and cached, keyed by a hash of the line's text, so repeated choruses share audio and editing a line only invalidates that line.
- **Quota-aware.** `gemini-3.8-flash-tts` allows 100 requests/day on Tier 1. Viewing lyrics, playing the MV and replaying cached audio cost nothing; only the first play of a clip uses TTS. Failed generations are not retried in a loop, and after a daily-quota error all clips are held until it resets.

## Getting started

Requirements: Python 3.12+ with [uv](https://docs.astral.sh/uv/), Node.js 20+, and a [Gemini API key](https://aistudio.google.com/apikey).

```bash
git clone https://github.com/kkdai/song-lingo.git
cd song-lingo
cp .env.example .env        # set GEMINI_API_KEY
uv sync

cd web
npm install
npm run dev                 # http://localhost:3000
```

Open the app, click **＋ 加入新歌 (Add song)**, and paste a YouTube MV link. Adding a song takes about a minute and uses roughly two Gemini Flash requests (no TTS quota).

### Using the study page

- Click a line to select it; double-click to play that line in the MV.
- Keyboard: `←` `→` previous / next line, `N` teacher (normal), `S` teacher (slow), `R` replay the original line.
- **✏️ 校對 (Review)** edits a line; **🔄 重新分析整首 (Re-analyze)** refreshes romanization and word breakdowns after edits (one Flash request; manual translations and review marks are kept).
- If a video's owner has disabled embedding, the player is replaced by the thumbnail and a link that opens YouTube at the current line.

## Command-line scripts

The web app runs these for you, but they also work on their own:

```bash
uv run transcribe.py "https://www.youtube.com/watch?v=VIDEO_ID"   # → output/VIDEO_ID.json
uv run annotate.py output/VIDEO_ID.json                            # → output/VIDEO_ID.annotated.json
uv run speak.py output/VIDEO_ID.annotated.json                     # pre-generate all teacher audio
```

`speak.py` spends one TTS request per clip (about 50–70 per song), so on Tier 1 it's usually better to let the web app generate audio as you study.

## Project structure

```
transcribe.py         YouTube MV → timestamped lyrics
annotate.py           romanization, translation, word breakdown, notes
speak.py              bulk teacher-audio generation
config/teachers.json  teacher voice descriptions and speaking styles (shared by Python and the web app)
web/                  Next.js app
output/               your songs and audio (git-ignored)
```

## Deployment

Deploying to Google Cloud Run — with audio and song data in a private Cloud Storage bucket and access restricted to a single Google account via Identity-Aware Proxy — is in progress.

## About the lyrics

Song lyrics belong to their rights holders. This repository contains no lyrics; everything you transcribe stays in your own `output/` folder or private bucket.
