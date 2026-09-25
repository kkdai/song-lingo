# song-lingo

Learn languages through songs: transcribe lyrics from a YouTube MV with Gemini, then study them line by line.

## Setup

```bash
cp .env.example .env   # add your GEMINI_API_KEY
uv sync
```

## Transcribe

```bash
uv run transcribe.py "https://www.youtube.com/watch?v=VIDEO_ID"
```

Writes `output/<VIDEO_ID>.json` with timestamped lines (plus hiragana readings for Japanese).
## Annotate

```bash
uv run annotate.py output/VIDEO_ID.json
```

Adds per line: Traditional Chinese translation, word breakdown (reading / part of speech / meaning),
grammar note, pronunciation tip, and romanization (Japanese: Hepburn from kana with particle fixes;
Korean: Revised Romanization). Japanese lines whose readings disagree between the two Gemini passes
are flagged `needs_review`. Writes `output/VIDEO_ID.annotated.json`.

## Speak

```bash
uv run speak.py output/VIDEO_ID.annotated.json
```

Generates teacher read-aloud clips with `gemini-3.8-flash-tts` — a normal and a slow version of
each unique line — into `output/audio/VIDEO_ID/`, and adds their paths to the annotated JSON.
A "teacher" voice per language is created once via voice design and cached in `output/voices.json`
(preview: `output/voice_preview_<lang>.wav`). Existing clips are skipped, so re-running only
retries failures.

## Web app

```bash
cd web
npm install
npm run dev   # http://localhost:3000
```

A Next.js app that reads songs from `../output/` (override with `SONG_DATA_DIR`). Each song page
shows the MV (YouTube embed) next to a line-by-line study card: furigana, romanization, Chinese
translation, word breakdown, grammar note, pronunciation tip, and teacher audio (normal / slow).
Click a line to select it, double-click to play that line in the MV. Keys: ← → switch line,
N / S teacher audio, R replay the original line.

Transcripts are for personal study and are git-ignored — lyrics are copyrighted, so don't commit or redistribute them.
