# Saved Shelf Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The desktop app saves its shelf (Ready to go, Later, Made before) in SQLite, restores it on launch, and resumes an interrupted clip from its finished download.

**Architecture:** A new `internal/shelf` package owns a `shelf.db` store. `appcore.Core` becomes the writer: it saves on Add/lookup/edits/lane changes and records run results itself, so a closed window still keeps results. `internal/clip` keeps a clip's downloaded section in `scratch\<id>\` with a fingerprint marker so a later run can skip the download. The page keeps a working copy in `state.js` and renders three sections.

**Tech Stack:** Go 1.26, `database/sql` + `modernc.org/sqlite` (pure Go, no cgo), Wails v2, vanilla JS modules tested with `node --test`.

**Spec:** `docs/superpowers/specs/2026-10-20-shelf-persistence-design.md`. Read it first; this plan names only what it fixes. Assumes branch `worktree-per-link-options` (per-link formats and quality) is the base.

## Global Constraints

- SQLite driver: `modernc.org/sqlite`; one open connection (`SetMaxOpenConns(1)`); WAL; `PRAGMA busy_timeout` of 5000 ms. CI and the build stay cgo-free.
- Versioning with `PRAGMA user_version` and an ordered list of migration funcs in code; no migration library.
- Store file `shelf.db` and folder `scratch\` live next to `settings.json` (`filepath.Dir(settingsPath)`), so `appcore.New`'s signature does not change.
- Clip IDs are unique across launches: 128-bit random hex from `shelf.NewID()`. The page treats IDs as opaque.
- Lanes are exactly `ready`, `later`, `made`. New links land in `later`. Make never reads or changes any clip outside `ready`.
- Clear made removes only `made` rows and never touches files. Clear cache removes `shelf.db` (+ `-wal`, `-shm`) and `scratch\`, never `settings.json` or the clips folder, and is refused while a run is going.
- A resumed clip never starts by itself on launch; an interrupted clip returns to `ready` with `interrupted` set.
- Resume rule: marker present and fingerprint equal → skip download; marker missing or different → delete the scratch folder and download fresh. Scratch is deleted whenever a run ends other than by cancel/crash, and when the clip is removed.
- `clip.Opts.ScratchDir == ""` must keep today's behaviour exactly (the CLI relies on it).
- Page rules: remote text only via `textContent`/attributes, never `innerHTML`; plain warm sentences, no tool names; every new control has an accessible name.
- Store failures never crash or block: log, show one notice, keep working in memory.
- Style: small packages under `internal/`, pure logic in `state.js`, Go tests beside the code. Commit after each task; do not push branches.

## Review Focus

Each line has its pinning test in the task named.

1. Window closed within the debounce after typing a time: `BeforeClose` asks the page to flush and waits for its ack, with a timeout when the page is gone (Task 4).
2. A made file deleted or moved, or the output folder changed since: the log line still loads, shows "File moved or deleted", and Make again works; previews work for every folder in the history (Task 3).
3. A stale page sends Make with ids that are in Later or Made (after Clear cache, or a race): Go filters to `ready` only (Task 4).
4. A playlist link replaced in place by N rows keeps position and order; the same link twice stays two clips (Task 1).
5. Corrupt, too-new, locked or unwritable `shelf.db`: moved aside or kept in memory with one notice, never a crash (Tasks 1 and 3).
6. Output folder or formats changed between an interruption and the next Make: the saved download is reused only when its fingerprint still matches (Task 2).

---

### Task 1: `internal/shelf` store

**Files:**
- Create: `internal/shelf/shelf.go`, `internal/shelf/migrate.go`, `internal/shelf/shelf_test.go`
- Modify: `go.mod`, `go.sum` (`go get modernc.org/sqlite`), `THIRD-PARTY-NOTICES.txt` (add an entry: modernc.org/sqlite, BSD-3-Clause, https://gitlab.com/cznic/sqlite, linked into vid2gif-app.exe)

**Interfaces:**
- Produces (types):
  - `type Lane string` with `LaneReady, LaneLater, LaneMade`.
  - `type File struct{ Format, Path string; Size int64 }` (json `format`, `path`, `size`).
  - `type Clip struct` with json (camelCase) fields: `ID, Lane, URL, Title, ThumbnailURL string; DurationSec float64; Source string; Unavailable bool; Reason string; Failed, Pending bool; Message, StartText, EndText, EndKind string; Start *float64; End float64; TimeMsg, Checked string; Formats []string; Quality string; ResultKind, ResultMessage string; Files []File; Interrupted bool; CreatedAt, MadeAt time.Time`, plus unexported-to-JSON `Making bool` (`json:"-"`).
- Produces (funcs): `func NewID() string`; `func Open(path string) (s *Store, notice string, err error)`; `(*Store).Close() error`; `Load() ([]Clip, error)` (ready, later, made in lane order, each in position order); `Put(c Clip) error` (upsert; a new ID is appended at the end of its lane); `Replace(oldID string, with []Clip) error` (rows take the old row's place, in order); `SetLane(ids []string, l Lane) error` (appended to the lane end, in the given order); `Remove(id string) error`; `MarkMaking(id string) error`; `Finish(id string, kind, message string, files []File) error` (lane `made`, `MadeAt` now, `making` cleared, `interrupted` cleared); `Interrupt(id string) error` (back to `ready`, `interrupted` set, `making` cleared); `ClearMade() error`; `IDs() (map[string]bool, error)`; `Wipe() error` (close, delete db + wal + shm, reopen empty).
- Notices (exact copy): open failure or too-new schema → `"Couldn't open your saved shelf. Starting fresh. A copy was kept."`

- [ ] **Step 1: Failing tests** in `shelf_test.go` (use `t.TempDir()`):
  `TestRoundTripEveryField` (Put a fully populated Clip incl. `Start` nil and non-nil, `Formats`, `Files`; Load returns equal),
  `TestNewLinksAppendAndLanesOrder`, `TestReplaceKeepsPositionAndOrder` (a placeholder between two clips replaced by 3 → order a, x1, x2, x3, b),
  `TestSameLinkTwiceIsTwoClips`, `TestSetLaneAppendsInGivenOrder`, `TestFinishMovesToMadeAndClearsFlags`,
  `TestOpenPutsMakingClipsBackInReadyInterrupted` (MarkMaking, Close, Open → lane ready, Interrupted true),
  `TestClearMadeKeepsReadyAndLater`, `TestWipeEmptiesStoreAndRemovesFiles`,
  `TestMigrationsRunOnceAndReopenIsIdempotent` (user_version equals len(migrations) after two opens),
  `TestCorruptFileIsMovedAsideAndReplaced` (write garbage; Open returns the exact notice, a `shelf.db.bad-*` file exists, store is empty and usable),
  `TestTooNewSchemaIsMovedAside` (set `user_version` to 999 via a raw connection first),
  `TestWriteFailureReturnsErrorNotPanic` (open a store, make the file read-only/closed via `Close()`, then `Put` returns an error).
- [ ] **Step 2:** `go test ./internal/shelf/` → FAIL (package missing).
- [ ] **Step 3: Implement** per Interfaces. `migrate.go`: `var migrations = []func(*sql.Tx) error{...}`; migration 1 creates `clips` with the columns above (`formats` and `files` as JSON text, `position` integer, `making`/`interrupted` integer). `Open` runs `PRAGMA integrity_check` after opening; any failure or `user_version > len(migrations)` renames the file to `shelf.db.bad-<unix>` and starts empty with the notice. All writes run in one transaction.
- [ ] **Step 4:** `go test ./internal/shelf/ && go vet ./internal/shelf/` → PASS.
- [ ] **Step 5: Commit** `feat: shelf store for saved clips`.

---

### Task 2: Resume from a finished download (`clip`, `progress`, `runner`)

**Files:**
- Modify: `internal/clip/clip.go` (`Opts`, `run`), `internal/progress/progress.go` (`Apply`), `internal/runner/runner.go` (`Job`, `runJob`)
- Test: `internal/clip/clip_test.go`, `internal/progress/progress_test.go`, `internal/runner/runner_test.go`

**Interfaces:**
- Produces: `Opts.ScratchDir string` (`""` = today's behaviour); `const StageResumed = "resumed"` (emitted instead of the first download report when a saved download is reused); `runner.Job.ScratchDir string` (`json:"-"`, set by Go only) passed through to `Opts.ScratchDir`.
- Behaviour with `ScratchDir` set: the download goes to `ScratchDir`; encode part files go to a fresh `os.MkdirTemp(OutDir, ".vid2gif-tmp-")` removed on every exit (the final rename must stay on the clips folder's volume). After yt-dlp succeeds and the clip file is picked, write `download.ok` containing a hash of `URL|Start|End|FullSource|wantAudio`. On entry: marker present and equal → skip download (emit `StageResumed`, use `pickClipFile(ScratchDir)`); otherwise `RemoveAll(ScratchDir)` then `MkdirAll`. On return: `RemoveAll(ScratchDir)` unless `ctx.Err() != nil`.

- [ ] **Step 1: Failing tests** in `clip_test.go` using the existing fake `Pipeline` fields:
  `TestScratchKeptOnCancelAndReusedByNextRun` (cancel in encode → folder + marker remain; second run's fake `download` is not called, a `StageResumed` event is emitted, outputs are written),
  `TestScratchWithoutMarkerIsRedownloaded`, `TestScratchWithDifferentFingerprintIsRedownloaded` (change `End`, and separately add `mp4` so `wantAudio` flips),
  `TestScratchRemovedAfterFinishEvenWhenAFormatFailed`, `TestScratchRemovedWhenDownloadFails`,
  `TestEmptyScratchDirKeepsOldBehaviour` (no folder created besides the OutDir temp, which is removed).
  In `progress_test.go`: `TestResumedPassesTheDownloadStage` (feeding `StageFind`, `StageResumed`, then an encode event never lowers `Overall()` and the download cost counts as done).
  In `runner_test.go`: `TestJobScratchDirReachesTheClipRunner`.
- [ ] **Step 2:** `go test ./internal/clip/ ./internal/progress/ ./internal/runner/` → FAIL.
- [ ] **Step 3: Implement** as described. `Tracker.Apply` handles `clip.StageResumed` by calling `t.At("download", 1)` and returning `ok=false`. Hash with `crypto/sha256`.
- [ ] **Step 4:** same command → PASS; also `go test ./...` (CLI tests unchanged).
- [ ] **Step 5: Commit** `feat: keep a finished download so an interrupted clip can resume`.

---

### Task 3: Persist the shelf in `appcore` (add, edit, lanes, boot)

**Files:**
- Modify: `internal/appcore/appcore.go`, `cmd/vid2gif-app/app.go`
- Create: `internal/appcore/shelf.go` (the methods below), `internal/appcore/shelf_test.go`
- Test: reuse `newTestCore`.

**Interfaces:**
- Consumes: Task 1 API.
- Produces on `Core`: store opened in `New` from `filepath.Dir(settingsPath)/shelf.db` (a failed open keeps working with an in-memory store, remembers the notice); `c.id()` now returns `shelf.NewID()`.
- `type ShelfClip struct { shelf.Clip; Files []PageFile \`json:"files"\`; FilesMissing bool \`json:"filesMissing"\` }` (outer `Files` shadows, as `Status` does).
- `type ShelfState struct { Clips []ShelfClip \`json:"clips"\`; Notice string \`json:"notice"\` }`; `BootState` gains `Shelf ShelfState`.
- Bound methods (also added to `App` in `cmd/vid2gif-app/app.go`): `Save(c shelf.Clip) error` (upsert; lane, result and making fields from the page are ignored — Go keeps its own), `SetLane(ids []string, lane string) error` (rejects `made`), `Remove(id string)` (also deletes `scratch\<id>`), `MakeAgain(id string) (shelf.Clip, error)` (new ID, lane `later`, same setup, no result), `ClearMade() error`.
- `Add` stores a `Pending` row in `later` for each placeholder; `lookupOne` calls `store.Replace` before it sends `slips:replace` (resolved rows copy the placeholder's `Formats`/`Quality`/`Lane` from the stored row). A `Pending` row found at boot is returned as `Failed` with `lookupMsg`.
- Boot: for each `made` clip, `Files.Allow(dir of each file)`, set `FilesMissing` when `c.stat` fails for any file, and fill `PageFile.URL` via `c.Files.URL`. Sweep: delete `scratch\<id>` folders with no row, and `.vid2gif-tmp-*` folders in the current `OutDir`.
- Store errors: log, and send one `shelf:notice` event `Message{...}` ("Couldn't save your shelf just now. It will keep working until you close the app.") the first time only.

- [ ] **Step 1: Failing tests** in `shelf_test.go`:
  `TestAddWritesPendingRowsAndReplaceSavesTheResolvedOnes` (fake `c.lookup` returning a 3-video playlist: Boot afterwards returns 3 clips in `later`, in order, with `Source`),
  `TestBootReturnsSavedLanesAndTypedTimes` (Save a clip with `StartText:"0:12"`, new `Core` on the same dir, Boot returns it unvalidated),
  `TestPendingAtBootBecomesRetryable`, `TestSetLaneRejectsMade`, `TestMakeAgainCreatesNewClipInLater`,
  `TestRemoveDeletesScratchFolder`, `TestClearMadeKeepsOtherLanes`,
  `TestBootMarksMissingFilesAndAllowsEveryHistoryFolder` (made clip whose file does not exist, and one in a second folder; the second folder's URL is issued),
  `TestBootSweepsOrphanScratch`, `TestCorruptStoreStillBoots` (garbage `shelf.db` → `Shelf.Notice` set, empty shelf),
  `TestStoreWriteFailureSendsOneNotice`.
- [ ] **Step 2:** `go test ./internal/appcore/` → FAIL.
- [ ] **Step 3: Implement.** Hold the store behind `c.mu`-free use (the store serialises itself). Keep `Make` untouched here.
- [ ] **Step 4:** `go test ./internal/appcore/ && go build ./...` → PASS. Also `go build -tags desktop,production -o NUL ./cmd/vid2gif-app` compiles.
- [ ] **Step 5: Commit** `feat: save and restore the shelf`.

---

### Task 4: Record runs, interruptions, resume wiring, Clear cache, flush

**Files:**
- Modify: `internal/appcore/appcore.go` (`Make`, `run`, `Stop`, `Shutdown`), `internal/appcore/shelf.go`, `cmd/vid2gif-app/app.go`, `cmd/vid2gif-app/main.go` (add `OnBeforeClose`)
- Test: `internal/appcore/shelf_test.go`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces: `(*Core).Make(jobs []runner.Job)` filters `jobs` to clips whose stored lane is `ready`, drops any not in the store, and sets each job's `ScratchDir` to `scratch\<id>` and `Formats`/`Quality` from the stored clip (not the page's). While running, the event wrapper calls `store.MarkMaking(id)` on a job's first `making` event, `store.Finish(...)` on `done|partial|failed|already`, and `store.Interrupt(id)` on `ready` (cancel). On `run` exit any job still `making` is `Interrupt`ed. `(*Core).ClearCache() error` (refuses with `"Wait for the clips to finish, or press Stop, before clearing."` while `c.cancel != nil`; otherwise `store.Wipe()`, remove `scratch\`, reset in-memory state, nothing else). `(*Core).BeforeClose(wait time.Duration)` emits `shelf:flush` and waits for `FlushDone()` or `wait`. `(*Core).FlushDone()`. Bound on `App`: `ClearCache() error`, `FlushDone()`; `main.go` sets `OnBeforeClose` to call `app.core.BeforeClose(1500*time.Millisecond)` and return false.

- [ ] **Step 1: Failing tests:**
  `TestMakeOnlyRunsReadyClips` (Later and Made ids in the job list are ignored; the fake clip runner sees only ready ones),
  `TestMakeUsesStoredFormatsAndQualityAndGivesScratchDir`,
  `TestFinishedClipsMoveToMadeWithoutAPage` (no event listener attached; after `run` returns the store has `made` rows with files),
  `TestStopReturnsCurrentClipToReadyInterrupted`,
  `TestKilledMidRunComesBackReadyAndInterrupted` (simulate: `MarkMaking`, drop the Core without finishing, new Core on same dir → ready + interrupted, and Make does not start by itself),
  `TestResumeSkipsDownloadAfterStop` (fake `clip.Pipeline`-level runner via existing `fakeClip`: first run cancelled after download leaves `scratch\<id>\download.ok`; second Make passes the same `ScratchDir`),
  `TestClearCacheRefusedWhileRunning`, `TestClearCacheWipesStoreAndScratchButNotSettingsOrOutput`,
  `TestBeforeCloseWaitsForFlushDoneAndTimesOutWithoutPage`.
- [ ] **Step 2:** `go test ./internal/appcore/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** `go test ./... && go vet ./...` → PASS.
- [ ] **Step 5: Commit** `feat: record results on the Go side and resume interrupted clips`.

---

### Task 5: Lanes in `state.js`

**Files:**
- Modify: `cmd/vid2gif-app/frontend/state.js`, `cmd/vid2gif-app/frontend/state.test.js`

**Interfaces (all pure, return new arrays):**
- Slips gain `lane` (`"ready" | "later" | "made"`), `interrupted`, `filesMissing`, `resultKind`.
- `fromShelf(saved)` → slips (a made clip gets `status = {kind: resultKind, message: resultMessage, files, stage:"", pct:1, stagePct:-1}`).
- `inLane(slips, lane)`; `madeVisible(slips, shown)` (newest first, `shown` count).
- `markReady(slips, ids)` moves only `isReady` slips (valid, validated, not pending/failed/unavailable) and returns them appended to the end of Ready; `markAllReady(slips)` applies it to all valid Later; `moveToLater(slips, id)`.
- `makeAgain(slips, oldId, fresh)` inserts Go's returned clip at the end of Later and leaves the made line.
- `applyStatus` also sets `lane:"made"` on `done|partial|failed|already` (and `interrupted:false`), and `interrupted:true` on `ready` (cancel).
- `readyJobs`, `needsCheck`, `makeCount` only look at `lane === "ready"`; `addPlaceholders` puts new slips in `later`.
- `stampFor(slip)` → `{label, tone}` from `status.kind`: `done|already`→`DONE`/`good`, `partial`→`PARTLY`/`bad`, `failed`→`FAILED`/`bad`.

- [ ] **Step 1: Failing tests** (one per function; key assertions): `readyJobs` ignores later and made slips; `needsCheck`/`makeCount` ignore them too; `markAllReady` moves only the valid Later ones in order and leaves a slip with no Start in Later; `moveToLater` clears nothing else; `applyStatus` `done` → lane made, `ready` → interrupted true and still ready; `makeAgain` leaves the original made slip untouched and appends the new one to Later; `madeVisible` newest first and capped; `fromShelf` round-trips a made clip with files and `filesMissing`; `stampFor` maps `already` to DONE and `partial` to PARTLY.
- [ ] **Step 2:** `node --test "cmd/vid2gif-app/frontend/*.test.js"` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** same command → PASS (existing tests updated only where they assumed one flat list).
- [ ] **Step 5: Commit** `feat: lanes in the page's shelf logic`.

---

### Task 6: Page: Ready to go and Later sections, saving

**Files:**
- Modify: `cmd/vid2gif-app/frontend/app.js`, `index.html`, `styles.css`, `dev-stub.js`

**Interfaces:**
- Consumes: Task 5 functions; Go methods `Save`, `SetLane`, `Remove`, `MakeAgain`, `FlushDone` and event `shelf:flush`, `shelf:notice`; `boot.shelf`.
- Behaviour:
  - `index.html`: replace the single `<ol class="shelf">` with three `<section>`s (`id="sec-ready"`, `"sec-later"`, `"sec-made"`), each with a heading (`Ready to go`, `Later`, `Made before`), a sticker count, and its own `<ol>` (the made one is a `<ul class="log">`, built in Task 7). An empty Ready or Later section shows one quiet line ("Nothing ready yet." / "New links land here."). Section styling follows `.superpowers` mockup `shelf-sections-v2`: `.sec-h` flex row, Bowlby heading 17px, `.slip.later { border-style: dashed }`. Slip rows in Later get a **Mark ready** button (disabled until the slip is ready-valid), a **Mark all ready** button sits in the Later header; Ready slips get **Move to Later**. The Make button label/count only reflect Ready.
  - Saving: every change to a slip's `startText/endText/endKind/formats/quality`, or a validation result, calls `App.Save(slip)` debounced 300 ms per slip (`ponytail:` comment: window closes inside the debounce are covered by the `shelf:flush` handler, ceiling = ack timeout); lane buttons call `App.SetLane` immediately; Remove calls `App.Remove`. The Make click first awaits every pending `Save`/`SetLane` promise (Go filters Make to stored `ready` clips, so the store must be current before `App.Make`). On `shelf:flush`, run all pending saves, await them, then call `App.FlushDone()`.
  - Boot: `slips = S.fromShelf(boot.shelf.clips)`; show `boot.shelf.notice` via the existing `notice()` if set; `shelf:notice` shows its message.
  - A slip with `interrupted` shows one line "Stopped partway. Make carries on."
  - `dev-stub.js`: `Boot()` returns a `shelf` with 2 ready, 2 later, 6 made (one failed, one with `filesMissing`); stub `Save/SetLane/Remove/MakeAgain/ClearMade/ClearCache/FlushDone` to mutate an in-memory list.
- [ ] **Step 1:** Write the markup, styles and handlers above (no unit tests: DOM code; logic is covered by Task 5).
- [ ] **Step 2:** `node --test "cmd/vid2gif-app/frontend/*.test.js"` → PASS.
- [ ] **Step 3: Verify in a browser** with the dev stub: `python -m http.server` in `cmd/vid2gif-app/frontend`, open `index.html`. Expected: three sections, Mark ready moves a valid slip up and disables itself for an invalid one, Mark all ready skips the one with no Start, Make count equals the Ready count, no console errors. If no browser tool is available, say so in the commit message and the final report.
- [ ] **Step 4: Commit** `feat: Ready to go and Later sections`.

---

### Task 7: Page: Made before log, Clear made, Clear cache, interruption note

**Files:**
- Modify: `cmd/vid2gif-app/frontend/app.js`, `index.html`, `styles.css`, `dev-stub.js`

**Behaviour:**
- Made before renders `S.madeVisible(slips, shown)` as the **return log** from `.superpowers` mockup `made-before-shelf` option B: one `<button class="row">` per clip with a 44px thumb (or the `scene-none` art), full title, `FORMATS · Quality · day` meta (day via `toLocaleDateString` with weekday), and a rotated stamp from `S.stampFor`. Initial `shown = 5`; a **Show more** button adds 25 and hides itself when all are shown. Clicking a row opens the existing inline preview (reuse `paintPreview`) when files exist and are not `filesMissing`; otherwise the row expands to "File moved or deleted." with **Make again** and **Remove**. Every row has **Make again** in its expanded panel; it calls `App.MakeAgain(id)` then `S.makeAgain`. Rows are real `<button>`s with `aria-expanded`.
- Made-before header has **Clear made** (`.btn.quiet`, no confirmation) calling `App.ClearMade()` then dropping `made` slips locally.
- Settings popover gets a "Clear cache" group below Helper files: note "Forgets your shelf and history. Your clips folder is not touched." and a **Clear cache** button that asks `confirm`-style inline ("Are you sure? This removes your ready and later clips too." with **Yes, clear** / **Cancel**, not `window.confirm`), disabled while `running`. On success: `slips = []`, `previewId = null`, render. On the Go error message, show it with `notice()`.
- [ ] **Step 1:** Implement the above.
- [ ] **Step 2:** `node --test "cmd/vid2gif-app/frontend/*.test.js"` → PASS.
- [ ] **Step 3: Verify in the browser with the dev stub:** log shows 5 then more; the failed row has a red FAILED stamp; a `filesMissing` row says "File moved or deleted." and still offers Make again; Clear made leaves Ready and Later; Clear cache's inline confirm empties everything; Tab/Enter/Space work on rows and buttons.
- [ ] **Step 4: Commit** `feat: Made before log, Clear made and Clear cache`.

---

### Task 8: Docs, installer check, manual checklist, full verification

**Files:**
- Modify: `docs/development.md` (a short "Saved shelf" note: where `shelf.db` and `scratch\` live, how to reset), `docs/building.md` only if the dependency changes the build command
- Create: `docs/manual-testing-shelf.md`
- Check: `installer/vid2gif.wxs` (comment at line ~11 says `%LOCALAPPDATA%\vid2gif` is removed on uninstall; confirm `shelf.db` and `scratch\` are inside it and update that comment to mention them)

- [ ] **Step 1:** Write `docs/manual-testing-shelf.md` with these checks: close mid-download (reopen: clip is in Ready, Make restarts the download); close mid-encode after the download finished (Make skips straight to encoding, progress bar does not go backwards); kill the process during a run; delete a made file and reopen; move the clips folder and reopen; damage `shelf.db` (notice shown, copy kept as `shelf.db.bad-*`); Clear made (Ready and Later stay, files stay); Clear cache (empty shelf at once, `settings.json` and clips untouched); type a time and close the window immediately; add 30 links, mark 5 ready, Make makes only 5.
- [ ] **Step 2:** Full verification, each must pass: `go build ./...`, `go vet ./...`, `go test ./...`, `node --test "cmd/vid2gif-app/frontend/*.test.js"`, and `go build -tags desktop,production -ldflags "-H windowsgui -s -w -X main.version=dev" -o dist/vid2gif-app.exe ./cmd/vid2gif-app` (confirm `dist/` is gitignored).
- [ ] **Step 3:** If a real window can be run, walk the manual checklist; otherwise say which items were not run.
- [ ] **Step 4: Commit** `docs: saved shelf notes and manual checklist`.
