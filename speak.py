"""Generate teacher read-aloud audio (normal + slow) for each unique line of an annotated song.

Usage:
    uv run speak.py output/<video_id>.annotated.json [--workers 4]

Designs one "teacher" voice per language on first use (cached in output/voices.json),
writes WAVs to output/audio/<video_id>/ and records their paths back into the annotated JSON.
"""

import argparse
import base64
import json
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from dotenv import load_dotenv
from google import genai

MODEL = "gemini-3.8-flash-tts"
VOICES_FILE = Path("output/voices.json")

TEACHERS = {
    "ja": dict(
        language_code="ja-JP",
        display_name="Song Lingo Japanese Teacher",
        description=(
            "A warm, patient Japanese language teacher in her early 30s from Tokyo. Standard "
            "Japanese accent, clear articulation, gentle and encouraging, like reading aloud to a student."
        ),
    ),
    "ko": dict(
        language_code="ko-KR",
        display_name="Song Lingo Korean Teacher",
        description=(
            "A warm, patient Korean language teacher in her early 30s from Seoul. Standard Seoul "
            "accent, clear articulation, gentle and encouraging, like reading aloud to a student."
        ),
    ),
    "en": dict(
        language_code="en-US",
        display_name="Song Lingo English Teacher",
        description=(
            "A warm, patient English teacher in her early 30s with a neutral American accent. "
            "Clear articulation, gentle and encouraging, like reading aloud to a student."
        ),
    ),
}

STYLES = {
    "normal": "calm and clear, reading the line aloud at a natural pace like a teacher",
    "slow": "speaking slowly and clearly, articulating every syllable for a beginner, with small pauses between words",
}


def teacher_voice(client: genai.Client, language: str) -> str:
    voices = json.loads(VOICES_FILE.read_text()) if VOICES_FILE.exists() else {}
    if language in voices:
        return voices[language]
    teacher = TEACHERS.get(language)
    if teacher is None:
        sys.exit(f"No teacher voice defined for language '{language}'.")

    print(f"Designing {language} teacher voice (one-time) ...")
    created = client.voices.create(
        store=True,
        voice={
            "model": MODEL,
            "type": "prompted",
            "display_name": teacher["display_name"],
            "gender": "female",
            "language_code": teacher["language_code"],
            "prompted": {"input": teacher["description"]},
        },
    )
    voices[language] = created.id
    VOICES_FILE.parent.mkdir(parents=True, exist_ok=True)
    VOICES_FILE.write_text(json.dumps(voices, indent=2))
    if created.sample_audio and created.sample_audio.data:
        preview = VOICES_FILE.parent / f"voice_preview_{language}.wav"
        preview.write_bytes(base64.b64decode(created.sample_audio.data))
        print(f"voice preview: {preview}")
    return created.id


def synthesize(client: genai.Client, voice_id: str, text: str, style: str) -> bytes:
    interaction = client.interactions.create(
        model=MODEL,
        input=[{
            "type": "user_input",
            "content": [{
                "type": "text",
                "text": text,
                "annotations": [{"type": "speech_metadata", "style": style}],
            }],
        }],
        response_format={"type": "audio"},
        generation_config={"speech_config": [{"voice": voice_id}]},
    )
    return base64.b64decode(interaction.output_audio.data)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("annotated", type=Path, help="Annotated JSON from annotate.py")
    parser.add_argument("--workers", type=int, default=4)
    args = parser.parse_args()

    load_dotenv()
    client = genai.Client()
    song = json.loads(args.annotated.read_text())
    voice_id = teacher_voice(client, song["language"])

    video_id = args.annotated.name.split(".")[0]
    audio_dir = args.annotated.parent / "audio" / video_id
    audio_dir.mkdir(parents=True, exist_ok=True)

    unique_texts = list(dict.fromkeys(line["text"] for line in song["lines"]))
    jobs = [
        (audio_dir / f"{i:03d}_{speed}.wav", text, style)
        for i, text in enumerate(unique_texts)
        for speed, style in STYLES.items()
    ]
    todo = [job for job in jobs if not job[0].exists()]
    print(f"{len(unique_texts)} unique lines -> {len(jobs)} clips ({len(jobs) - len(todo)} cached, {len(todo)} to generate)")

    def run(job):
        path, text, style = job
        try:
            path.write_bytes(synthesize(client, voice_id, text, style))
            return None
        except Exception as e:  # keep going; failed clips are retried on the next run
            return f"{path.name}: {e}"

    with ThreadPoolExecutor(max_workers=args.workers) as pool:
        errors = [e for e in pool.map(run, todo) if e]

    audio_by_text = {
        text: {speed: str(audio_dir / f"{i:03d}_{speed}.wav") for speed in STYLES}
        for i, text in enumerate(unique_texts)
    }
    for line in song["lines"]:
        line["audio"] = audio_by_text[line["text"]]
    args.annotated.write_text(json.dumps(song, ensure_ascii=False, indent=2))

    print(f"generated: {len(todo) - len(errors)}  failed: {len(errors)}")
    for e in errors[:5]:
        print(f"  {e}")
    if errors:
        print("Re-run to retry failed clips.")
    print(f"audio dir: {audio_dir}")


if __name__ == "__main__":
    main()
