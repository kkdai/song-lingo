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

## 部署

正在進行中：部署到 Google Cloud Run，歌曲資料和音檔放在私人的 Cloud Storage bucket，並用 Identity-Aware Proxy 限制只有指定的 Google 帳號能存取。

## 關於歌詞

歌詞的權利屬於原權利人。本 repo 不包含任何歌詞，你轉錄的內容只會存在自己的 `output/` 資料夾或私人的 bucket 裡。
