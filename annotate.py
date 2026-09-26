"""Annotate a transcript with romanization, Traditional Chinese translation and word/grammar notes.

Usage:
    uv run annotate.py output/<video_id>.json [--model MODEL]

Writes output/<video_id>.annotated.json next to the input.
"""

import argparse
import json
import sys
from pathlib import Path

import pykakasi
from dotenv import load_dotenv
from google import genai
from google.genai import types
from pydantic import BaseModel, Field

DEFAULT_MODEL = "gemini-3.8-flash"

PROMPT = """You are a friendly language teacher helping a Taiwanese learner study a {language_name} song.
Below are the unique lines of the song, each with an index. For EVERY line, return one entry with the
same index.

For each line:
- translation_zh: natural Traditional Chinese (Taiwan) translation that keeps the line's feeling.
- tokens: {token_rule}
- romanization: {romanization_rule}
- grammar_note: one short note in Traditional Chinese about the most useful grammar point or
  expression in this line (empty string if nothing notable).
- pronunciation_tip: one short tip in Traditional Chinese about how to pronounce this line
  (e.g. long vowels, sound changes, linking); empty string if nothing notable.

Lines:
{lines}
"""

LANG_RULES = {
    "ja": dict(
        language_name="Japanese",
        token_rule=(
            "split the line into words in order, covering the whole line. For each token give "
            "surface (exactly as written), reading (hiragana, as sung), pos (one of: noun, verb, "
            "adjective, adverb, particle, auxiliary, pronoun, conjunction, interjection, other) and "
            "meaning_zh (short Traditional Chinese gloss)."
        ),
        romanization_rule="empty string (computed separately).",
    ),
    "ko": dict(
        language_name="Korean",
        token_rule=(
            "split the line into words (eojeol) in order, covering the whole line. For each token "
            "give surface, reading (empty string), pos, and meaning_zh (short Traditional Chinese gloss)."
        ),
        romanization_rule=(
            "Revised Romanization of Korean for the whole line, applying actual pronunciation "
            "rules (liaison, nasalization, etc.), e.g. 감사합니다 -> gamsahamnida."
        ),
    ),
}
DEFAULT_RULES = dict(
    language_name="{language}",
    token_rule=(
        "only the notable vocabulary, idioms or phrasal verbs in the line (can be empty). Give "
        "surface, reading (empty string), pos, and meaning_zh."
    ),
    romanization_rule="empty string.",
)


class Token(BaseModel):
    surface: str
    reading: str = ""
    pos: str = ""
    meaning_zh: str = ""


class LineNote(BaseModel):
    index: int
    translation_zh: str
    tokens: list[Token]
    romanization: str = ""
    grammar_note: str = ""
    pronunciation_tip: str = ""


class Annotations(BaseModel):
    lines: list[LineNote] = Field(description="One entry per input line, same index")


# Particles whose kana spelling differs from how they are pronounced.
PARTICLE_ROMAJI = {"は": "wa", "へ": "e", "を": "o"}
# Set phrases that end in a fossilized particle は.
WORD_ROMAJI = {"こんにちは": "konnichiwa", "こんばんは": "konbanwa"}
_kakasi = pykakasi.kakasi()


def kana_to_romaji(kana: str) -> str:
    return "".join(item["hepburn"] for item in _kakasi.convert(kana))


def romanize_ja(tokens: list[Token]) -> str:
    words = []
    for token in tokens:
        kana = token.reading or token.surface
        if kana in WORD_ROMAJI:
            words.append(WORD_ROMAJI[kana])
        elif token.pos == "particle" and kana in PARTICLE_ROMAJI:
            words.append(PARTICLE_ROMAJI[kana])
        else:
            words.append(kana_to_romaji(kana))
    return " ".join(w for w in words if w.strip())


def annotate(client: genai.Client, texts: list[str], language: str, model: str) -> tuple[Annotations | None, str]:
    rules = LANG_RULES.get(language, {k: v.format(language=language) for k, v in DEFAULT_RULES.items()})
    lines = "\n".join(f"{i}: {text}" for i, text in enumerate(texts))
    response = client.models.generate_content(
        model=model,
        contents=PROMPT.format(lines=lines, **rules),
        config=types.GenerateContentConfig(
            response_mime_type="application/json",
            response_schema=Annotations,
            temperature=0.2,
            automatic_function_calling=types.AutomaticFunctionCallingConfig(disable=True),
        ),
    )
    finish = response.candidates[0].finish_reason if response.candidates else None
    return response.parsed, finish.name if finish else "NO_CANDIDATES"


def strip_spaces(s: str) -> str:
    return "".join(s.split())


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("transcript", type=Path, help="Transcript JSON from transcribe.py")
    parser.add_argument("--model", default=DEFAULT_MODEL)
    args = parser.parse_args()

    load_dotenv()
    transcript = json.loads(args.transcript.read_text())
    language = transcript["language"]
    lines = transcript["lines"]

    # Choruses repeat, so annotate each unique line once and map back.
    unique_texts = list(dict.fromkeys(line["text"] for line in lines))
    print(f"Annotating {len(unique_texts)} unique lines ({len(lines)} total, language={language}) ...")

    result, finish = annotate(genai.Client(), unique_texts, language, args.model)
    if finish == "RECITATION":
        sys.exit("Blocked by Gemini (finish_reason=RECITATION).")
    if result is None:
        sys.exit(f"No parseable annotations (finish_reason={finish}).")

    notes = {note.index: note for note in result.lines}
    missing = [i for i in range(len(unique_texts)) if i not in notes]
    token_mismatch = 0
    by_text = {}
    for i, text in enumerate(unique_texts):
        note = notes.get(i)
        if note is None:
            continue
        entry = note.model_dump(exclude={"index"})
        if language == "ja":
            note.romanization = entry["romanization"] = romanize_ja(note.tokens)
            # Transcribe and annotate produce readings independently; disagreement flags a likely misread kanji.
            token_reading = strip_spaces("".join(t.reading for t in note.tokens))
            line_reading = strip_spaces(next(l.get("reading") or "" for l in lines if l["text"] == text))
            entry["needs_review"] = token_reading != line_reading
        if language in LANG_RULES and strip_spaces("".join(t.surface for t in note.tokens)) != strip_spaces(text):
            token_mismatch += 1
        by_text[text] = entry

    for line in lines:
        line.update(by_text.get(line["text"], {}))
        # Keep what the user fixed in the review UI.
        line.pop("stale", None)
        if line.get("translation_zh_manual"):
            line["translation_zh"] = line["translation_zh_manual"]
        if line.get("reviewed"):
            line["needs_review"] = False
            line["uncertain"] = False

    out_file = args.transcript.with_suffix(".annotated.json")
    out_file.write_text(json.dumps(transcript, ensure_ascii=False, indent=2))

    print(f"finish_reason: {finish}")
    print(f"annotated: {len(unique_texts) - len(missing)}/{len(unique_texts)}  missing indexes: {missing}")
    if language in LANG_RULES:
        print(f"lines where tokens don't cover the text: {token_mismatch}")
    if language == "ja":
        print(f"lines needing review: {sum(bool(l.get('needs_review')) for l in lines)}")
    print(f"saved: {out_file}")


if __name__ == "__main__":
    main()
