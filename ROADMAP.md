# Roadmap

**English** | [繁體中文](#繁體中文)

Ideas for where Song Lingo goes next, grounded in what the code does today. Nothing here is promised; items are grouped by theme and roughly ordered by value within each group.

Constraints every item has to respect:

- **Personal, private use.** Lyrics are copyrighted — nothing may make transcripts public or shareable outside the owner's own storage.
- **Quota-aware.** `gemini-3.8-flash-tts` allows ~100 requests/day on Tier 1. New features should reuse cached audio and existing annotations before spending new requests.
- **Single instance.** Job status, clip dedupe and quota back-off live in memory (`web/lib/tts.ts`, `web/lib/review.ts`), and data is plain JSON on disk or a bucket mount. Anything that adds shared state should fit that model or replace it deliberately.

## 1. Remember what I've learned

✅ **Shipped:** per-song progress (Firestore on Cloud Run, `output/progress/<videoId>.json` locally; `web/lib/progress.ts`) — resume at the last line, learned lines, shadowing score history with frequently missed words, and progress on the home page. Remaining:

- **Personal word list** — tap a word in the breakdown (`Word` in `SongStudy.tsx`) to save it with its reading, meaning and the line it came from. Can live in the same progress file.
- **"Lines I keep missing" across songs** — the shadowing history already stores missed words per attempt; aggregate them into one review list.
- **Spaced-repetition review** — a daily review page that quizzes saved words and hard lines (show the line, hide the translation; play the teacher clip, type what you hear). Uses only cached audio and existing annotations, so it costs no quota.
- **Anki / CSV export** of the word list, with the line as the example sentence.

## 2. Better practice tools

- **A-B loop and slow playback of the original** — loop the current line of the MV, and use the YouTube player's `setPlaybackRate` (0.75× / 0.5×) to hear the singer slowly, not only the teacher.
- **Sub-second timestamps** — transcription returns `MM:SS`, so line boundaries can be up to a second off. Ask for `MM:SS.s`, or add a one-tap "nudge start/end" control in the review form.
- **Karaoke mode** — hide the lyrics (or blank out a few words) while the MV plays and fill in the gap; a listening-comprehension drill built from data that already exists.
- **Sing-along shadowing** — score a recording made over the original music, not just a spoken reading (harder; may need the vocal-only track or a looser match).
- **Pitch-accent hints for Japanese** — shadowing deliberately ignores pitch accent today. Show the accent pattern for each word (e.g. from a dictionary such as OJAD data) as a study aid, without scoring it.
- **Grammar-point index** — collect every `grammar_note` across all songs into one searchable page, so "～ても" links to every line that uses it.

## 3. Library and song management

- **Delete a song** — there is no delete endpoint; removing a song today means deleting files from `output/` (and its audio folder) by hand.
- **Search and filter** on the home page by title, artist, language and progress.
- **Edit title/artist** — `title_guess` / `artist_guess` come from Gemini and can't be corrected in the UI.
- **Playlists / collections** — e.g. "JLPT N3 songs", "this week".
- **Difficulty estimate** per song (vocabulary level, speed, line count) to pick what to study next.
- **Import lyrics I already have** — paste official lyrics and let Gemini only align timestamps, skipping transcription errors entirely.

## 4. More languages and learners

- **More song languages** — Mandarin and Cantonese (pinyin / jyutping), Spanish, French, Thai. The pipeline is already keyed by language (`LANG_RULES` in `annotate.py`, `config/teachers.json`, `LANGUAGE_CODES` in `shadow.py`), so each language is mostly prompt rules, a teacher voice and a romanization step.
- **Configurable explanation language** — translations and notes are hard-coded to Traditional Chinese (Taiwan). Making the target language a setting would open the app to non-Chinese-speaking learners.
- **Interface i18n** — the UI strings are Chinese only.
- **Choose a teacher** — more than one voice per language (male/female, regional accent) from `config/teachers.json`.

## 5. Quota and cost

- **Quota dashboard** — show today's TTS and transcribe usage and when the limit resets, instead of discovering it on the first failed clip.
- **Pre-generate during idle time** — optionally spend leftover daily TTS quota on the next song's clips, most-studied lines first.
- **Batch annotate on edits** — re-annotate only stale lines instead of the whole song.

## 6. Engineering

- **Tests** — there are none yet. Start with the pure functions that carry the most logic: `romanize_ja` and the reading cross-check in `annotate.py`, `char_states` / `token_statuses` in `shadow.py`, and `editLine` in `web/lib/review.ts`.
- **Split `SongStudy.tsx`** — at ~850 lines it holds the player sync, keyboard shortcuts, shadowing and review; extracting the line card, shadow panel and mobile controls would make new practice features easier to add.
- **Schema versioning** for the song JSON so new fields (progress, accent data) can be migrated.
- **Durable job state** — move add-song status and quota back-off out of memory if the app ever needs more than one instance or should survive restarts mid-job.
- **CI** — lint, type-check and the test suite on every pull request.

---

## 繁體中文

以目前程式碼為基礎，整理 Song Lingo 接下來可以做的功能。以下都不是承諾，依主題分組，組內大致依價值排序。

每個功能都要遵守的前提：

- **個人、私人使用。** 歌詞有版權，任何功能都不能讓歌詞公開或分享到使用者自己的儲存空間以外。
- **節省配額。** `gemini-3.8-flash-tts` 在 Tier 1 每天約 100 次。新功能應優先重用已快取的音檔與既有標註，再考慮花新的請求。
- **單一執行個體。** 工作狀態、音檔去重與配額退避都存在記憶體（`web/lib/tts.ts`、`web/lib/review.ts`），資料是磁碟或 bucket 上的 JSON。新增共享狀態時要符合這個模式，或有意識地替換它。

### 1. 記住我學過什麼

✅ **已完成：** 每首歌的學習進度（Cloud Run 上存 Firestore，本機存 `output/progress/<videoId>.json`；`web/lib/progress.ts`）：回到上次那一句、已學會的句子、跟讀分數紀錄與常念不好的詞、首頁顯示進度。尚未完成：

- **個人單字本** — 在逐字解析（`SongStudy.tsx` 的 `Word`）點一下就收藏，連同讀音、意思和出處那一句。可以存在同一個進度檔。
- **跨歌曲的「常念錯」清單** — 跟讀紀錄已經保存每次沒念好的詞，可以彙整成一份複習清單。
- **間隔重複複習** — 每日複習頁，測驗收藏的單字與困難的句子（顯示歌詞、隱藏翻譯；播放老師音檔、聽寫）。只用已快取的音檔與既有標註，不花配額。
- **匯出 Anki / CSV** — 單字本匯出，以歌詞當例句。

### 2. 更好的練習工具

- **原曲 A-B 循環與慢速播放** — 循環 MV 的目前這句，並用 YouTube 播放器的 `setPlaybackRate`（0.75× / 0.5×）慢速聽歌手唱，而不只是聽老師唸。
- **更精準的時間軸** — 轉錄只回傳 `MM:SS`，句子邊界可能差到一秒。改要求 `MM:SS.s`，或在校對表單加「微調開始／結束」按鈕。
- **卡拉 OK 模式** — MV 播放時隱藏歌詞（或挖空幾個字）讓使用者填空，用現有資料做聽力練習。
- **跟著原曲唱的跟讀** — 對著原曲伴奏錄音並評分，不只是唸（較難，可能需要人聲分離或較寬鬆的比對）。
- **日文高低音調提示** — 跟讀目前刻意不檢查音調。可為每個字顯示重音型（例如參考 OJAD 等辭典資料），作為學習輔助而不評分。
- **文法點索引** — 把所有歌的 `grammar_note` 集中成一個可搜尋的頁面，例如「～ても」連到每一句用到它的歌詞。

### 3. 歌曲庫管理

- **刪除歌曲** — 目前沒有刪除 API，要刪歌只能手動刪 `output/` 裡的檔案和音檔資料夾。
- **首頁搜尋與篩選** — 依歌名、歌手、語言與進度。
- **修改歌名／歌手** — `title_guess` / `artist_guess` 是 Gemini 猜的，UI 無法修正。
- **歌單／分類** — 例如「JLPT N3 歌曲」、「本週」。
- **難度評估** — 依詞彙程度、速度、句數估計難度，幫忙挑下一首。
- **匯入現成歌詞** — 貼上官方歌詞，Gemini 只負責對齊時間軸，完全避開轉錄錯誤。

### 4. 更多語言與學習者

- **更多歌曲語言** — 華語與粵語（拼音／粵拼）、西班牙文、法文、泰文。流程已經依語言區分（`annotate.py` 的 `LANG_RULES`、`config/teachers.json`、`shadow.py` 的 `LANGUAGE_CODES`），新增語言主要是提示規則、老師聲音與拼音轉換。
- **可設定的解說語言** — 翻譯與註解目前固定為繁體中文（台灣）。改成設定值就能服務不懂中文的學習者。
- **介面多語系** — UI 目前只有中文。
- **選擇老師** — 每種語言提供多個聲音（男聲／女聲、不同口音），由 `config/teachers.json` 設定。

### 5. 配額與成本

- **配額儀表板** — 顯示今天 TTS 與轉錄用量、何時重置，而不是等第一個音檔失敗才知道。
- **閒置時預先產生** — 可選擇把當天剩下的 TTS 配額拿來產生下一首歌的音檔，最常練的句子優先。
- **只重新分析修改過的句子** — 只對 stale 的句子重跑標註，而不是整首。

### 6. 工程

- **測試** — 目前沒有。先從邏輯最多的純函式開始：`annotate.py` 的 `romanize_ja` 與讀音交叉檢查、`shadow.py` 的 `char_states` / `token_statuses`、`web/lib/review.ts` 的 `editLine`。
- **拆分 `SongStudy.tsx`** — 約 850 行，同時處理播放同步、快捷鍵、跟讀與校對；把句子卡片、跟讀面板與行動版控制抽出來，之後加練習功能會更容易。
- **歌曲 JSON 的 schema 版本** — 讓新欄位（進度、音調資料）可以遷移。
- **持久化工作狀態** — 若需要多個執行個體或要在工作途中重啟，把加歌狀態與配額退避移出記憶體。
- **CI** — 每個 pull request 跑 lint、型別檢查與測試。
