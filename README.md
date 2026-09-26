# Song Lingo

**English** | [繁體中文](README_tw.md)

<img width="1307" height="962" alt="Google Chrome 2026-09-26 12 13 29" src="https://github.com/user-attachments/assets/5d5971af-b842-485b-b1df-c910400b1263" />


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

## Deployment (Google Cloud Run, private to you)

Song Lingo runs on Cloud Run as a single container: the Next.js app plus the uv-managed Python pipeline it calls. Everything is locked down so that only the Google accounts you list can use it — the page shows full lyrics and every generation spends your Gemini quota, so it should never be public.

| Piece | Setup |
|---|---|
| Container | `Dockerfile` (Node 22 + uv/Python), built by Cloud Build from source |
| Song data and audio | A private Cloud Storage bucket mounted at `/data` (`SONG_DATA_DIR`) |
| API key | Secret Manager, exposed as `GEMINI_API_KEY` |
| Access | Identity-Aware Proxy (IAP) in front, plus an in-app check of IAP's signed header |
| Scaling | One instance (`--max-instances=1`), CPU always allocated so "add song" jobs finish in the background |

Why a single instance: clip-generation dedupe, the quota back-off and add-song job status live in memory, and the bucket mount has no cross-instance locking. That's plenty for personal use.

### Four layers of access control

1. **Cloud Run IAM** — `--no-allow-unauthenticated`; only the IAP service agent may invoke the service.
2. **IAP** — only accounts granted `roles/iap.httpsResourceAccessor` get through.
3. **In-app check** — `web/proxy.ts` verifies the `x-goog-iap-jwt-assertion` header (ES256, issuer, audience) on every request and compares the email with `ALLOWED_EMAILS`. It only runs on Cloud Run (`K_SERVICE` is set) and **fails closed**: if `IAP_AUDIENCE` or `ALLOWED_EMAILS` is missing, every request gets a 500.
4. **Private bucket** — public access prevention enforced; only the service's own service account can read or write it. Audio is streamed through the app, never via signed URLs.

### Steps

Replace `PROJECT_ID`, `PROJECT_NUMBER`, `REGION`, `BUCKET` and `you@gmail.com`.

```bash
# 1. Private bucket
gcloud storage buckets create gs://BUCKET --project=PROJECT_ID --location=REGION \
  --uniform-bucket-level-access --public-access-prevention

# 2. A dedicated service account with bucket- and secret-scoped roles only
gcloud iam service-accounts create song-lingo-run --project=PROJECT_ID
SA=song-lingo-run@PROJECT_ID.iam.gserviceaccount.com
gcloud storage buckets add-iam-policy-binding gs://BUCKET \
  --member=serviceAccount:$SA --role=roles/storage.objectUser

printf '%s' "$GEMINI_API_KEY" | gcloud secrets create song-lingo-gemini-api-key \
  --project=PROJECT_ID --data-file=-
gcloud secrets add-iam-policy-binding song-lingo-gemini-api-key --project=PROJECT_ID \
  --member=serviceAccount:$SA --role=roles/secretmanager.secretAccessor

# 3. (Optional) Upload songs you already have locally
gcloud storage rsync output gs://BUCKET --recursive --exclude='.*\.tmp$'

# 4. Build and deploy
gcloud run deploy song-lingo --source . --project=PROJECT_ID --region=REGION \
  --no-allow-unauthenticated --iap \
  --service-account=$SA \
  --set-secrets=GEMINI_API_KEY=song-lingo-gemini-api-key:latest \
  --set-env-vars=ALLOWED_EMAILS=you@gmail.com,IAP_AUDIENCE=/projects/PROJECT_NUMBER/locations/REGION/services/song-lingo \
  --max-instances=1 --min-instances=0 --no-cpu-throttling \
  --execution-environment=gen2 --memory=1Gi --cpu=1 --timeout=600 \
  --add-volume=name=data,type=cloud-storage,bucket=BUCKET \
  --add-volume-mount=volume=data,mount-path=/data

# 5. Let your account through IAP
gcloud iap web add-iam-policy-binding --project=PROJECT_ID \
  --member=user:you@gmail.com --role=roles/iap.httpsResourceAccessor \
  --resource-type=cloud-run --region=REGION --service=song-lingo
```

`.gcloudignore` keeps `.env` and `output/` out of the Cloud Build upload; check with `gcloud meta list-files-for-upload .` before your first deploy.

### Projects without an organization (personal Gmail)

IAP's Google-managed OAuth client only works for accounts inside an organization. If your project has no organization (`gcloud projects describe PROJECT_ID --format='value(parent)'` prints nothing), IAP answers every request with `502 Empty Google Account OAuth client ID(s)/secret(s)` until you add your own OAuth client:

1. **Google Auth Platform** (OAuth consent screen): audience **External**. If it's in *Testing*, add yourself as a test user. If it's already *In production* (for example shared with other apps in the project), leave it as is — IAP and the in-app check still decide who gets in.
2. **APIs & Services → Credentials → Create OAuth client ID → Web application.** Add the redirect URI `https://iap.googleapis.com/v1/oauth/clientIds/CLIENT_ID:handleRedirect`.
3. Attach it to the service (run this in your own terminal so the secret stays out of logs and chats):

```bash
cat > /tmp/iap-oauth.yaml <<'EOF'
accessSettings:
  oauthSettings:
    clientId: CLIENT_ID
    clientSecret: CLIENT_SECRET
EOF
gcloud iap settings set /tmp/iap-oauth.yaml --project=PROJECT_ID \
  --resource-type=cloud-run --region=REGION --service=song-lingo > /dev/null
rm /tmp/iap-oauth.yaml
```

### Verify it's private

| Check | Expected |
|---|---|
| `curl -I https://SERVICE_URL/` | `302` to `accounts.google.com` (from IAP) |
| Same with a forged `x-goog-iap-jwt-assertion` header | Still `302` — IAP doesn't accept assertions from outside |
| `curl https://storage.googleapis.com/BUCKET/voices.json` | `403` |
| `gcloud run services get-iam-policy song-lingo` | Only the IAP service agent, no `allUsers` |
| Sign in with your account | App works |
| Sign in with another Google account | "You don't have access" |
| Logs for `[auth] rejected` | None for your own requests (a wrong `IAP_AUDIENCE` shows up here) |

Also worth doing: restrict the API key to the Generative Language API, and set a billing budget alert.

To redeploy after code changes, `gcloud run deploy song-lingo --source . --project=PROJECT_ID --region=REGION` reuses the existing settings. Note that the bucket and your local `output/` are separate copies; use `gcloud storage rsync` to move data between them.

## About the lyrics

Song lyrics belong to their rights holders. This repository contains no lyrics; everything you transcribe stays in your own `output/` folder or private bucket.
