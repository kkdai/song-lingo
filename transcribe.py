"""Transcribe song lyrics (with timestamps) from a public YouTube MV using Gemini.

Usage:
    uv run transcribe.py <youtube_url> [--model MODEL] [--out DIR]
"""

import argparse
import json
import re
import sys
from pathlib import Path

from dotenv import load_dotenv
from google import genai
from google.genai import types
from pydantic import BaseModel, Field

DEFAULT_MODEL = "gemini-3.8-flash"

PROMPT = """You are a professional lyric transcriber. Listen to the vocals in this music video
and transcribe the sung lyrics line by line, with timestamps.

Rules:
- If the video shows on-screen lyrics/subtitles in the original language, use them as the
  primary source (they give the correct kanji/spelling). Otherwise transcribe from the audio.
- Write lyrics in the original script (e.g. Japanese kanji/kana, Korean hangul). Do not translate.
- For Japanese lines, fill `reading` with the full hiragana reading of the line.
- One entry per sung line/phrase. Repeat choruses every time they are sung.
- Exclude non-lyric content: intros, spoken ads, credits, instrumental-only sections.
- Set `uncertain` to true for any line you could not hear clearly or had to guess.
- Timestamps are MM:SS relative to the start of the video.
"""


class LyricLine(BaseModel):
    start: str = Field(description="Start time, MM:SS")
    end: str = Field(description="End time, MM:SS")
    text: str = Field(description="Lyric line in original script")
    reading: str | None = Field(default=None, description="Hiragana reading (Japanese only)")
    section: str | None = Field(default=None, description="e.g. verse, pre-chorus, chorus, bridge")
    uncertain: bool = False


class Transcript(BaseModel):
    language: str = Field(description="ISO 639-1 code of the main sung language, e.g. ja, ko")
    title_guess: str | None = None
    artist_guess: str | None = None
    source: str = Field(description="'onscreen' if on-screen lyrics were used, 'audio' otherwise, or 'mixed'")
    lines: list[LyricLine]


YOUTUBE_ID = re.compile(r"(?:v=|youtu\.be/|/shorts/|/embed/)([\w-]{11})")


def video_id(url: str) -> str:
    m = YOUTUBE_ID.search(url)
    if not m:
        sys.exit(f"Not a recognizable YouTube URL: {url}")
    return m.group(1)


def transcribe(client: genai.Client, url: str, model: str) -> tuple[Transcript | None, str]:
    response = client.models.generate_content(
        model=model,
        contents=types.Content(
            parts=[
                types.Part(file_data=types.FileData(file_uri=url)),
                types.Part(text=PROMPT),
            ]
        ),
        config=types.GenerateContentConfig(
            response_mime_type="application/json",
            response_schema=Transcript,
            temperature=0.2,
            automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
        ),
    )
    finish = response.candidates[0].finish_reason if response.candidates else None
    finish_name = finish.name if finish else "NO_CANDIDATES"
    return response.parsed, finish_name


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("url", help="Public YouTube video URL")
    parser.add_argument("--model", default=DEFAULT_MODEL)
    parser.add_argument("--out", default="output", type=Path)
    args = parser.parse_args()

    load_dotenv()
    vid = video_id(args.url)
    client = genai.Client()  # reads GEMINI_API_KEY

    print(f"Transcribing {vid} with {args.model} ...")
    transcript, finish = transcribe(client, f"https://www.youtube.com/watch?v={vid}", args.model)

    if finish == "RECITATION":
        sys.exit("Blocked by Gemini (finish_reason=RECITATION): the model refused to reproduce these lyrics.")
    if transcript is None:
        sys.exit(f"No parseable transcript (finish_reason={finish}).")

    args.out.mkdir(parents=True, exist_ok=True)
    out_file = args.out / f"{vid}.json"
    out_file.write_text(json.dumps(transcript.model_dump(), ensure_ascii=False, indent=2))

    uncertain = sum(line.uncertain for line in transcript.lines)
    print(f"finish_reason: {finish}")
    print(f"language: {transcript.language}  source: {transcript.source}")
    print(f"guess: {transcript.artist_guess} - {transcript.title_guess}")
    print(f"lines: {len(transcript.lines)}  uncertain: {uncertain}")
    print(f"saved: {out_file}")


if __name__ == "__main__":
    main()
