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

## Teacher audio

Teacher read-aloud clips (`gemini-3.8-flash-tts`, normal + slow) are generated **on demand**: the
web app synthesizes a line the first time it is played and caches it in `output/audio/VIDEO_ID/`.
Each clip costs one TTS request (Tier 1 allows 100/day), so only lines you actually practice use quota.

To pre-generate a whole song instead:

```bash
uv run speak.py output/VIDEO_ID.annotated.json
```

Teacher voices and speaking styles live in `config/teachers.json` (shared by `speak.py` and the web
app). One voice per language is created via voice design on first use and cached in `output/voices.json`.

## Web app

```bash
cd web
npm install
npm run dev   # http://localhost:3000
```

A Next.js app that reads songs from `../output/` (override with `SONG_DATA_DIR`) and the API key from
the repo-root `.env`. Each song page
shows the MV (YouTube embed) next to a line-by-line study card: furigana, romanization, Chinese
translation, word breakdown, grammar note, pronunciation tip, and teacher audio (normal / slow).
Add songs from the **＋ 加入新歌** page: paste a YouTube URL and the app runs `transcribe.py` and
`annotate.py` in the background (about 2 Gemini Flash requests, no TTS quota), then opens the song.
If annotation fails after transcribing, adding the same URL again resumes from annotation.

Click a line to select it, double-click to play that line in the MV. Keys: ← → switch line,
N / S teacher audio, R replay the original line.

Transcripts are for personal study and are git-ignored — lyrics are copyrighted, so don't commit or redistribute them.
