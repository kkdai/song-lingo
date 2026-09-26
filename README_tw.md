# Song Lingo

[English](README.md) | **繁體中文**

用你喜歡的歌學語言。貼上 YouTube MV 的網址，Song Lingo 會轉錄歌詞，加上拼音、中文翻譯和文法說明，還有一位老師用正常速度或慢速逐句念給你聽，讓你一句一句把整首歌學起來。

> 歌詞受著作權保護。Song Lingo 是**個人學習工具**：轉錄的歌詞只存在你自己的電腦（或你私人的雲端儲存空間），已經被 git 忽略，請不要 commit 或散布。

## 主要功能

- **貼 YouTube 網址加入新歌**：Gemini 會看 MV，並逐句轉錄歌詞和時間軸。MV 畫面上有歌詞字幕時，會優先採用字幕。
- **逐句教學卡片**：漢字上方標假名、羅馬拼音、繁體中文翻譯、逐字拆解（讀音、詞性、意思），每句再加一個文法重點和一個發音提示。
- **老師逐句示範**：用 Gemini TTS 的 voice design，從一段文字描述設計出老師的聲音。每句都有正常速度和「慢慢念、每個音節都清楚」兩種版本。
- **跟著 MV 學**：歌詞列表會跟著影片播放自動切換，也可以只重播原曲的某一句。
- **校對與修正**：可能有錯的句子會被標記出來，可以直接修改原文、讀音或翻譯，再重新分析整首歌。
- **支援語言**：日文、韓文（附羅馬拼音）和英文（附單字與連音說明）。

## 使用的技術

| 部分 | 技術 |
|---|---|
| 歌詞轉錄 | `gemini-3.8-flash` 直接讀取 YouTube 網址，以結構化 JSON 輸出 |
| 翻譯、單字拆解、文法說明 | `gemini-3.8-flash` |
| 老師的聲音 | `gemini-3.8-flash-tts`：voice design（用文字描述產生聲音）與逐句的語氣控制 |
| 羅馬拼音 | Gemini 負責斷詞與詞性；[pykakasi](https://github.com/miurahr/pykakasi) 把假名轉成平文式拼音，並修正助詞念法（は→wa、へ→e、を→o）；韓文使用 Revised Romanization |
| 處理腳本 | Python 3.12、[uv](https://docs.astral.sh/uv/)、[google-genai](https://github.com/googleapis/python-genai) |
| 網頁 | [Next.js](https://nextjs.org/) 16（App Router）、React 19、Tailwind CSS 4 |
| 影片 | YouTube IFrame Player API |

## 運作方式

```
YouTube 網址
  → transcribe.py   歌詞與時間軸（日文另附平假名讀音）
  → annotate.py     羅馬拼音、繁中翻譯、逐字拆解、文法與發音說明
  → 網頁            在 MV 旁邊逐句學習
                    老師示範音在第一次播放時產生，之後直接使用存好的檔案
```

幾個設計上的重點：

- **用兩次獨立的讀音互相比對**：轉錄和分析時，Gemini 各自產生一次日文讀音。兩次對不上的句子會被標記為需要校對，不用多花任何 API 呼叫。
- **示範音按需產生**：每段音檔在第一次播放時才產生並存起來，檔名是句子內容的雜湊值。重複的副歌會共用同一段音檔；修改某句歌詞時，也只有那一句需要重新產生。
- **注意額度**：`gemini-3.8-flash-tts` 在 Tier 1 每天只能呼叫 100 次。看歌詞、播放 MV、重播已經產生過的示範音都不會消耗額度，只有第一次播放某段示範音時才會用到 TTS。產生失敗時不會一直重試；每日額度用完後，所有示範音都會暫停產生，直到額度重置。

## 開始使用

需求：Python 3.12 以上與 [uv](https://docs.astral.sh/uv/)、Node.js 20 以上，以及一組 [Gemini API key](https://aistudio.google.com/apikey)。

```bash
git clone https://github.com/kkdai/song-lingo.git
cd song-lingo
cp .env.example .env        # 填入 GEMINI_API_KEY
uv sync

cd web
npm install
npm run dev                 # 打開 http://localhost:3000
```

打開網頁後按「**＋ 加入新歌**」，貼上 YouTube MV 的網址。加入一首歌大約需要一分鐘，約使用 2 次 Gemini Flash 請求，不占 TTS 額度。

### 學習頁面的操作

- 單擊選擇一句，雙擊播放 MV 裡的那一句。
- 快捷鍵：`←` `→` 上一句、下一句，`N` 老師正常速度，`S` 老師慢速，`R` 重播原曲這一句。
- **✏️ 校對** 可以修改這一句。修改後按 **🔄 重新分析整首**，會更新拼音和單字拆解，使用 1 次 Flash 請求，手動改過的翻譯和校對標記都會保留。
- 如果影片擁有者不允許在其他網站播放，影片區塊會改成顯示縮圖，並提供一個連結，在 YouTube 上從目前這一句開始播放。

## 命令列腳本

網頁會自動執行這些腳本，不過它們也可以單獨使用：

```bash
uv run transcribe.py "https://www.youtube.com/watch?v=VIDEO_ID"   # → output/VIDEO_ID.json
uv run annotate.py output/VIDEO_ID.json                            # → output/VIDEO_ID.annotated.json
uv run speak.py output/VIDEO_ID.annotated.json                     # 一次產生整首歌的示範音
```

`speak.py` 每段音檔用 1 次 TTS 請求，一首歌大約 50–70 次。在 Tier 1 的額度下，通常讓網頁在學習時按需產生會比較划算。

## 專案結構

```
transcribe.py         YouTube MV → 附時間軸的歌詞
annotate.py           拼音、翻譯、逐字拆解、說明
speak.py              一次產生整首歌的示範音
config/teachers.json  老師聲音的描述與語氣設定（Python 和網頁共用）
web/                  Next.js 網頁
output/               你的歌曲與音檔（已被 git 忽略）
```

## 部署（Google Cloud Run，只有你能使用）

Song Lingo 在 Cloud Run 上以單一容器執行，裡面包含 Next.js 網頁和它會呼叫的 Python 處理腳本（用 uv 管理）。整個服務都鎖起來，只有你指定的 Google 帳號能使用：頁面上有完整的歌詞，而且每次產生內容都會消耗你的 Gemini 額度，所以絕對不應該公開。

| 部分 | 設定 |
|---|---|
| 容器 | `Dockerfile`（Node 22 + uv/Python），由 Cloud Build 從原始碼建置 |
| 歌曲資料與音檔 | 私人的 Cloud Storage bucket，掛載到 `/data`（`SONG_DATA_DIR`） |
| API key | 放在 Secret Manager，以 `GEMINI_API_KEY` 環境變數提供 |
| 存取控制 | 前面是 Identity-Aware Proxy（IAP），程式內再驗證一次 IAP 的簽章 |
| 執行個體 | 只有 1 個（`--max-instances=1`），CPU 持續分配，讓「加入新歌」的背景工作能跑完 |

只用 1 個執行個體的原因：避免重複產生音檔、額度用完時暫停呼叫，以及加入新歌的進度，這些狀態都存在記憶體裡；掛載的 bucket 也沒有跨執行個體的鎖定機制。個人使用，1 個執行個體就足夠。

### 四層存取控制

1. **Cloud Run 權限**：`--no-allow-unauthenticated`，只有 IAP 的服務帳號能呼叫這個服務。
2. **IAP**：只有被授予 `roles/iap.httpsResourceAccessor` 的帳號能通過。
3. **程式內驗證**：`web/proxy.ts` 會在每個請求上驗證 `x-goog-iap-jwt-assertion` 標頭（ES256 簽章、issuer、audience），並比對 email 是否在 `ALLOWED_EMAILS` 裡。只在 Cloud Run 上啟用（偵測 `K_SERVICE` 環境變數），而且**設定漏掉時一律拒絕**：沒有設定 `IAP_AUDIENCE` 或 `ALLOWED_EMAILS` 時，所有請求都回傳 500。
4. **私人 bucket**：強制禁止公開存取，只有這個服務專用的服務帳號能讀寫。音檔一律經過程式傳送，不使用簽署網址（signed URL）。

### 步驟

請把 `PROJECT_ID`、`PROJECT_NUMBER`、`REGION`、`BUCKET` 和 `you@gmail.com` 換成你自己的值。

```bash
# 1. 建立私人 bucket
gcloud storage buckets create gs://BUCKET --project=PROJECT_ID --location=REGION \
  --uniform-bucket-level-access --public-access-prevention

# 2. 建立專用服務帳號，只授予這個 bucket 和這個 secret 的權限
gcloud iam service-accounts create song-lingo-run --project=PROJECT_ID
SA=song-lingo-run@PROJECT_ID.iam.gserviceaccount.com
gcloud storage buckets add-iam-policy-binding gs://BUCKET \
  --member=serviceAccount:$SA --role=roles/storage.objectUser

printf '%s' "$GEMINI_API_KEY" | gcloud secrets create song-lingo-gemini-api-key \
  --project=PROJECT_ID --data-file=-
gcloud secrets add-iam-policy-binding song-lingo-gemini-api-key --project=PROJECT_ID \
  --member=serviceAccount:$SA --role=roles/secretmanager.secretAccessor

# 3.（選用）上傳本機已經有的歌曲
gcloud storage rsync output gs://BUCKET --recursive --exclude='.*\.tmp$'

# 4. 建置並部署
gcloud run deploy song-lingo --source . --project=PROJECT_ID --region=REGION \
  --no-allow-unauthenticated --iap \
  --service-account=$SA \
  --set-secrets=GEMINI_API_KEY=song-lingo-gemini-api-key:latest \
  --set-env-vars=ALLOWED_EMAILS=you@gmail.com,IAP_AUDIENCE=/projects/PROJECT_NUMBER/locations/REGION/services/song-lingo \
  --max-instances=1 --min-instances=0 --no-cpu-throttling \
  --execution-environment=gen2 --memory=1Gi --cpu=1 --timeout=600 \
  --add-volume=name=data,type=cloud-storage,bucket=BUCKET \
  --add-volume-mount=volume=data,mount-path=/data

# 5. 允許你的帳號通過 IAP
gcloud iap web add-iam-policy-binding --project=PROJECT_ID \
  --member=user:you@gmail.com --role=roles/iap.httpsResourceAccessor \
  --resource-type=cloud-run --region=REGION --service=song-lingo
```

`.gcloudignore` 會讓 `.env` 和 `output/` 不被上傳到 Cloud Build。第一次部署前，可以用 `gcloud meta list-files-for-upload .` 確認實際會上傳哪些檔案。

### 不屬於組織的專案（個人 Gmail）

IAP 預設的 OAuth client 只支援組織內的帳號。如果你的專案不屬於任何組織（`gcloud projects describe PROJECT_ID --format='value(parent)'` 沒有輸出），在你設定自己的 OAuth client 之前，IAP 會對所有請求回傳 `502 Empty Google Account OAuth client ID(s)/secret(s)`：

1. **Google Auth Platform**（OAuth 同意畫面）：目標對象選 **External（外部）**。如果發布狀態是「測試中」，把自己加進測試使用者。如果已經是「正式版」（例如和專案裡的其他應用程式共用），維持原樣即可，決定誰能使用的是 IAP 和程式內的驗證。
2. **API 和服務 → 憑證 → 建立 OAuth 用戶端 ID → 網頁應用程式**，並加入重新導向 URI `https://iap.googleapis.com/v1/oauth/clientIds/CLIENT_ID:handleRedirect`。
3. 把它套用到這個服務。請在你自己的終端機執行，避免用戶端密鑰出現在 log 或對話紀錄裡：

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

### 確認只有你能存取

| 檢查 | 預期結果 |
|---|---|
| `curl -I https://SERVICE_URL/` | IAP 回傳 `302`，導向 `accounts.google.com` |
| 同上，但附上偽造的 `x-goog-iap-jwt-assertion` 標頭 | 一樣是 `302`，IAP 不接受外部帶進來的簽章 |
| `curl https://storage.googleapis.com/BUCKET/voices.json` | `403` |
| `gcloud run services get-iam-policy song-lingo` | 只有 IAP 的服務帳號，沒有 `allUsers` |
| 用你的帳號登入 | 可以正常使用 |
| 用其他 Google 帳號登入 | 顯示「You don't have access」 |
| 在 log 裡搜尋 `[auth] rejected` | 你自己的請求不應該出現；如果 `IAP_AUDIENCE` 填錯，會在這裡看到 |

另外也建議：把 API key 限制成只能呼叫 Generative Language API，並設定帳單的預算警示。

修改程式後重新部署，執行 `gcloud run deploy song-lingo --source . --project=PROJECT_ID --region=REGION` 即可，其他設定都會沿用。要注意 bucket 和本機的 `output/` 是兩份各自獨立的資料，需要時可以用 `gcloud storage rsync` 在兩者之間同步。

## 關於歌詞

歌詞的權利屬於原權利人。本 repo 不包含任何歌詞，你轉錄的內容只會存在自己的 `output/` 資料夾或私人的 bucket 裡。
