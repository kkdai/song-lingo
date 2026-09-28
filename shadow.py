"""Score a shadowing attempt: transcribe the learner's recording and compare it with the line.

Usage:
    uv run shadow.py AUDIO_PATH MIME_TYPE EXPECTED_JSON

EXPECTED_JSON holds {"language", "text", "reading", "tokens": [{"surface", "reading"}]} for the line.
Prints a JSON result to stdout. On failure exits non-zero with a one-line reason on stderr
(never the transcript); a daily-quota error is reported as "QUOTA <retry seconds> <message>".
"""

import json
import sys
import unicodedata
from difflib import SequenceMatcher

import pykakasi
from dotenv import load_dotenv

MODEL = "gemini-3.5-transcribe"
LANGUAGE_CODES = {"ja": "ja-JP", "ko": "ko-KR", "en": "en-US"}
_kakasi = pykakasi.kakasi()


def to_hiragana(text: str) -> str:
    return "".join(item["hira"] for item in _kakasi.convert(text))


def fold(text: str) -> str:
    """Drop punctuation and spacing; fold width/case so only the spoken content is compared."""
    text = unicodedata.normalize("NFKC", text).lower()
    return "".join(ch for ch in text if not unicodedata.category(ch).startswith(("P", "S", "Z")) and not ch.isspace())


def char_states(expected: str, actual: str) -> list[str]:
    """Align the transcript to `expected`; per expected character: "equal", "replace" or "delete"."""
    states = ["delete"] * len(expected)
    for tag, i1, i2, _, _ in SequenceMatcher(None, expected, actual, autojunk=False).get_opcodes():
        if tag in ("equal", "replace"):
            states[i1:i2] = [tag] * (i2 - i1)
    return states


def token_statuses(parts: list[str], states: list[str]) -> list[str]:
    """ok = every character heard; missing = none of it was said; wrong = said, but differently."""
    statuses, pos = [], 0
    for part in parts:
        span = states[pos : pos + len(part)]
        pos += len(part)
        if all(st == "equal" for st in span):
            statuses.append("ok")
        elif all(st == "delete" for st in span):
            statuses.append("missing")
        else:
            statuses.append("wrong")
    return statuses


def score(expected: dict, transcript: str) -> dict:
    language = expected["language"]
    if language == "ja":
        tokens = expected.get("tokens") or [{"surface": expected["text"], "reading": expected.get("reading") or ""}]
        surfaces = [t["surface"] for t in tokens]
        readings = [to_hiragana(fold(t.get("reading") or t["surface"])) for t in tokens]
        written = [fold(s) for s in surfaces]
        # Compare both by reading (transcripts may write a word in kana or different kanji)
        # and by written form (guards against pykakasi misreading a kanji); either match counts.
        by_reading = token_statuses(readings, char_states("".join(readings), to_hiragana(fold(transcript))))
        by_surface = token_statuses(written, char_states("".join(written), fold(transcript)))
        statuses = ["ok" if "ok" in pair else pair[0] for pair in zip(by_reading, by_surface)]
        weights = [max(len(r), 1) for r in readings]
    else:
        surfaces = [w for w in expected["text"].split() if fold(w)]
        target = [fold(w) for w in surfaces]
        spoken = [fold(w) for w in transcript.split() if fold(w)]
        statuses = ["wrong"] * len(target)
        for tag, i1, i2, _, _ in SequenceMatcher(None, target, spoken, autojunk=False).get_opcodes():
            if tag in ("equal", "delete"):
                statuses[i1:i2] = ["ok" if tag == "equal" else "missing"] * (i2 - i1)
        weights = [1] * len(surfaces)

    total = sum(weights) or 1
    correct = sum(w for w, s in zip(weights, statuses) if s == "ok")
    return {
        "transcript": transcript,
        "score": round(100 * correct / total),
        "words": [{"surface": s, "status": st} for s, st in zip(surfaces, statuses)],
    }


def transcribe(audio_path: str, mime_type: str, language: str) -> str:
    from google import genai

    client = genai.Client()
    # A daily-quota 429 carries a Retry-After of hours; fail fast instead of sleeping on it.
    client.interactions.sdk_configuration.retry_config.max_retries = 0
    uploaded = client.files.upload(file=audio_path, config={"mime_type": mime_type})
    try:
        interaction = client.interactions.create(
            model=MODEL,
            input=[{"type": "audio", "uri": uploaded.uri, "mime_type": uploaded.mime_type}],
            generation_config={
                "transcription_config": {
                    "language_codes": [LANGUAGE_CODES.get(language, "en-US")],
                    # Verbatim, not smart: we want what was said, not a cleaned-up guess.
                    "mode": {"type": "verbatim", "timestamp_granularities": ["word"]},
                }
            },
        )
    finally:
        client.files.delete(name=uploaded.name)
    # The REST payload carries text in steps[].content[] (output_text is an SDK convenience).
    return "".join(
        content.text or ""
        for step in interaction.steps or []
        if step.type == "model_output"
        for content in step.content or []
        if content.type == "text"
    )


def main() -> None:
    if len(sys.argv) != 4:
        sys.exit("usage: shadow.py AUDIO_PATH MIME_TYPE EXPECTED_JSON")
    audio_path, mime_type, expected_path = sys.argv[1:]
    load_dotenv()
    expected = json.loads(open(expected_path, encoding="utf-8").read())
    try:
        transcript = transcribe(audio_path, mime_type, expected["language"])
    except Exception as e:
        if type(e).__name__ == "RateLimitError":
            response = getattr(e, "raw_response", None) or getattr(e, "response", None)
            retry = response.headers.get("retry-after", "0") if response is not None else "0"
            print(f"QUOTA {retry} {str(e).splitlines()[0][:300]}", file=sys.stderr)
            sys.exit(3)
        print(f"{type(e).__name__}: {str(e).splitlines()[0][:300] if str(e) else ''}", file=sys.stderr)
        sys.exit(2)
    if not transcript.strip():
        print("沒有聽到聲音，請靠近麥克風再念一次。", file=sys.stderr)
        sys.exit(4)
    print(json.dumps(score(expected, transcript), ensure_ascii=False))


if __name__ == "__main__":
    main()
