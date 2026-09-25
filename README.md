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

Transcripts are for personal study and are git-ignored — lyrics are copyrighted, so don't commit or redistribute them.
