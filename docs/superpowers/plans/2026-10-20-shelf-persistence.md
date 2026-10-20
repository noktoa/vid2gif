# Saved Shelf Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The desktop app saves its shelf (Ready to go, Later, Made before) in SQLite, restores it on launch, and resumes an unfinished clip from its finished download.

**Architecture:** A new `internal/shelf` package owns a `shelf.db` store. `appcore.Core` is the writer: it saves resolved lookups, page edits and run results itself, so a closed window still keeps results. A clip leaves Ready only when it finishes, so a crash or Stop needs no recovery. `internal/clip` keeps a clip's downloaded section in `scratch\<id>\` with a fingerprint marker so a later run can skip the download. The page keeps a working copy in `state.js` and renders three sections.

**Tech Stack:** Go 1.26, `database/sql` + `modernc.org/sqlite` (pure Go, no cgo), Wails v2, vanilla JS modules tested with `node --test`.

**Spec:** `docs/superpowers/specs/2026-10-20-shelf-persistence-design.md`. Read it first; this plan names only what it fixes. Base: branch `worktree-per-link-options` (per-link formats and quality).

## Global Constraints

- SQLite driver: `modernc.org/sqlite`; one open connection (`SetMaxOpenConns(1)`); WAL; `PRAGMA busy_timeout` of 5000 ms. CI and the build stay cgo-free.
- Versioning with `PRAGMA user_version` and an ordered list of migration funcs in code; no migration library.
- Store file `shelf.db` and folder `scratch\` live next to `settings.json` (`filepath.Dir(settingsPath)`), so `appcore.New`'s signature does not change.
- Clip IDs are unique across launches: 128-bit random hex (`shelf.NewID()`); the page makes its own for Make again with `crypto.randomUUID()`. IDs are opaque.
- Lanes are exactly `ready`, `later`, `made`. New links land in `later`. Only Go moves a clip to `made`. Make never reads or changes any clip outside `ready`.
- Only resolved lookups are saved; a failed or pending lookup is not.
- Every page edit is saved on change with no debounce.
- Clear made removes only `made` rows and never touches files. Clear cache removes `shelf.db` (+ `-wal`, `-shm`) and `scratch\`, never `settings.json` or the clips folder, and is refused while a run is going.
- Resume rule: marker present and its fingerprint text equal → skip the download; otherwise delete the scratch folder and download fresh. `clip` only keeps scratch; `appcore` deletes it when a clip finishes, is removed, or on Clear cache.
- `clip.Opts.ScratchDir == ""` must keep today's behaviour exactly (the CLI relies on it). The progress package is not changed.
- Page rules: remote text only via `textContent`/attributes, never `innerHTML`; plain warm sentences, no tool names; every new control has an accessible name.
- Store failures never crash or block: log, show one notice, keep working in memory.
- Style: small packages under `internal/`, pure logic in `state.js`, Go tests beside the code. Commit after each task; do not push branches.

## Review Focus

Each line has its pinning test in the task named.

1. A stale page sends Make with ids that are in Later or Made (after Clear cache, or a race): Go filters to `ready` only (Task 4).
2. A made file deleted or moved, or the output folder changed since: the log line still loads, shows "File moved or deleted", and Make again works; previews work for every folder in the history (Task 3).
3. A playlist of N videos keeps its order, and the same link added twice stays two clips (Task 1).
4. Corrupt, too-new, locked or unwritable `shelf.db`: moved aside or kept in memory with one notice, never a crash (Tasks 1 and 3).
5. Formats changed between an unfinished run and the next Make (audio needed or not): the saved download is reused only when its fingerprint still matches (Task 2).

---

### Task 1: `internal/shelf` store

**Files:**
- Create: `internal/shelf/shelf.go`, `internal/shelf/migrate.go`, `internal/shelf/shelf_test.go`
- Modify: `go.mod`, `go.sum` (`go get modernc.org/sqlite`), `THIRD-PARTY-NOTICES.txt` (add: modernc.org/sqlite, BSD-3-Clause, https://gitlab.com/cznic/sqlite, linked into vid2gif-app.exe)

**Interfaces:**
- Produces types: `type Lane string` with `LaneReady, LaneLater, LaneMade`; `type File struct{ Format, Path string; Size int64 }` (json `format`, `path`, `size`); `type Clip struct` with camelCase json fields `ID, Lane, URL, Title, ThumbnailURL string; DurationSec float64; Source string; Unavailable bool; Reason string; StartText, EndText, EndKind string; Start *float64; End float64; TimeMsg, Checked string; Formats []string; Quality string; ResultKind, ResultMessage string; Files []File; CreatedAt, MadeAt time.Time`.
- Produces funcs: `NewID() string`; `Open(path string) (s *Store, notice string, err error)`; `(*Store).Close() error`; `Load() ([]Clip, error)` (lanes ready, later, made; each in position order); `Put(c Clip) error` (upsert; a new ID, or an existing one whose lane changed, goes to the end of its lane); `Finish(id, kind, message string, files []File) error` (lane `made`, `MadeAt` now); `Remove(id string) error`; `ClearMade() error`; `Wipe() error` (close, delete db + wal + shm, reopen empty).
- Notice (exact copy) when the file can't be opened or is too new: `"Couldn't open your saved shelf. Starting fresh. A copy was kept."`

- [ ] **Step 1: Failing tests** in `shelf_test.go` (`t.TempDir()`):
  `TestRoundTripEveryField` (Start nil and non-nil, Formats, Files),
  `TestPlaylistRowsKeepOrderAndSameLinkTwiceIsTwoClips`,
  `TestPutOfExistingClipWithNewLaneMovesItToTheEnd`,
  `TestFinishMovesToMadeWithResult`,
  `TestClearMadeKeepsReadyAndLater`, `TestWipeEmptiesStoreAndRemovesFiles`,
  `TestMigrationsRunOnceAndReopenIsIdempotent` (`user_version` equals `len(migrations)` after two opens),
  `TestCorruptFileIsMovedAsideAndReplaced` (garbage file → exact notice, a `shelf.db.bad-*` file exists, store empty and usable),
  `TestTooNewSchemaIsMovedAside` (`user_version` 999),
  `TestWriteAfterCloseReturnsErrorNotPanic`.
- [ ] **Step 2:** `go test ./internal/shelf/` → FAIL (package missing).
- [ ] **Step 3: Implement** per Interfaces. `migrate.go`: `var migrations = []func(*sql.Tx) error{...}`; migration 1 creates `clips` (`formats`, `files` as JSON text; `position` integer). On open, a failure or `user_version > len(migrations)` renames the file to `shelf.db.bad-<unix>` and starts empty with the notice. Each write is one transaction.
- [ ] **Step 4:** `go test ./internal/shelf/ && go vet ./internal/shelf/` → PASS.
- [ ] **Step 5: Commit** `feat: shelf store for saved clips`.

---

### Task 2: Keep a finished download (`clip`, `runner`)

**Files:**
- Modify: `internal/clip/clip.go` (`Opts`, `run`), `internal/runner/runner.go` (`Job`, `runJob`)
- Test: `internal/clip/clip_test.go`, `internal/runner/runner_test.go`

**Interfaces:**
- Produces: `Opts.ScratchDir string` (`""` = today's behaviour); `runner.Job.ScratchDir string` (`json:"-"`) passed to `Opts.ScratchDir`.
- With `ScratchDir` set: the download goes to `ScratchDir`; encode part files go to a fresh `os.MkdirTemp(OutDir, ".vid2gif-tmp-")` removed on every exit (the rename into the clips folder must stay on its volume). After yt-dlp succeeds and the clip file is picked, write `download.ok` containing the plain fingerprint text `URL|Start|End|FullSource|wantAudio`. On entry: marker present and equal → skip the download and use `pickClipFile(ScratchDir)`; otherwise `RemoveAll(ScratchDir)` then `MkdirAll`. `clip` never deletes `ScratchDir` on exit.

- [ ] **Step 1: Failing tests** (existing fake `Pipeline` fields): in `clip_test.go`
  `TestScratchKeptAndReusedByNextRun` (cancel during encode → folder + marker remain; second run's fake `download` is not called, outputs are written),
  `TestScratchWithoutMarkerIsRedownloaded`,
  `TestScratchWithDifferentFingerprintIsRedownloaded` (change `End`; separately add `mp4` so `wantAudio` flips),
  `TestEmptyScratchDirKeepsOldBehaviour` (no extra folder; the OutDir temp is removed).
  In `runner_test.go`: `TestJobScratchDirReachesTheClipRunner`.
- [ ] **Step 2:** `go test ./internal/clip/ ./internal/runner/` → FAIL.
- [ ] **Step 3: Implement** as described.
- [ ] **Step 4:** `go test ./...` → PASS (CLI tests unchanged).
- [ ] **Step 5: Commit** `feat: keep a finished download so an unfinished clip can resume`.

---

### Task 3: Persist the shelf in `appcore` (add, edit, remove, boot)

**Files:**
- Modify: `internal/appcore/appcore.go`, `cmd/vid2gif-app/app.go`
- Create: `internal/appcore/shelf.go`, `internal/appcore/shelf_test.go`
- Test: reuse `newTestCore`.

**Interfaces:**
- Consumes: Task 1.
- Produces on `Core`: the store opened in `New` from `filepath.Dir(settingsPath)/shelf.db` (a failed open keeps working with an in-memory store and remembers the notice); `c.id()` returns `shelf.NewID()`.
- `type ShelfClip struct { shelf.Clip; Files []PageFile \`json:"files"\`; FilesMissing bool \`json:"filesMissing"\` }` (outer `Files` shadows, as `Status` does); `type ShelfState struct { Clips []ShelfClip \`json:"clips"\`; Notice string \`json:"notice"\` }`; `BootState` gains `Shelf ShelfState`.
- Bound methods (also on `App`): `Save(clips []shelf.Clip) error` (lane must be `ready` or `later`, else error; for an existing row Go keeps its own result fields; a new row is allowed, e.g. Make again), `Remove(id string)` (also deletes `scratch\<id>`), `ClearMade() error`.
- `Add` snapshots `c.Current().Formats` and `.Quality` for its placeholders; `lookupOne` saves each resolved clip to `later` with that snapshot (with `Source`, `StartText` from the link) before sending `slips:replace`. Failed or pending lookups are not saved.
- Boot: for each `made` clip, `Files.Allow(dir of each file)`, set `FilesMissing` when `c.stat` fails for any file, and fill `PageFile.URL` via `c.Files.URL`.
- Store errors: log, and send one `shelf:notice` event `Message{"Couldn't save your shelf just now. It will keep working until you close the app."}` the first time only.

- [ ] **Step 1: Failing tests** in `shelf_test.go`:
  `TestLookupSavesResolvedClipsInOrderToLater` (fake `c.lookup` returns a 3-video playlist; a new `Core` on the same dir boots with 3 `later` clips in order, with `Source` and the Add-time formats/quality),
  `TestFailedLookupIsNotSaved`,
  `TestBootReturnsSavedLanesAndTypedTimes` (Save a clip with `StartText:"0:12"`; reboot returns it unvalidated),
  `TestSaveRejectsMadeLaneAndKeepsExistingResult`,
  `TestSaveAcceptsANewRowForMakeAgain`,
  `TestRemoveDeletesScratchFolder`, `TestClearMadeKeepsOtherLanes`,
  `TestBootMarksMissingFilesAndAllowsEveryHistoryFolder` (a made clip whose file is gone, and one in a second folder whose URL is issued),
  `TestCorruptStoreStillBoots` (garbage `shelf.db` → `Shelf.Notice` set, empty shelf),
  `TestStoreWriteFailureSendsOneNotice`.
- [ ] **Step 2:** `go test ./internal/appcore/` → FAIL.
- [ ] **Step 3: Implement.** `Make` is untouched here.
- [ ] **Step 4:** `go test ./internal/appcore/ && go build ./...`, and `go build -tags desktop,production -o NUL ./cmd/vid2gif-app` → PASS.
- [ ] **Step 5: Commit** `feat: save and restore the shelf`.

---

### Task 4: Record runs, resume wiring, Clear cache

**Files:**
- Modify: `internal/appcore/appcore.go` (`Make`, `run`), `internal/appcore/shelf.go`, `cmd/vid2gif-app/app.go`
- Test: `internal/appcore/shelf_test.go`

**Interfaces:**
- Consumes: Tasks 1–3.
- Produces: `(*Core).Make(jobs []runner.Job)` keeps only jobs whose stored lane is `ready` (drops ids that are missing or in another lane) and, for each, sets `ScratchDir` to `scratch\<id>` and `Formats`/`Quality` from the stored clip, not the page's. The event wrapper calls `store.Finish` (then removes `scratch\<id>`) on `done|partial|failed|already`; a `ready` (cancel) event changes nothing in the store. `(*Core).ClearCache() error` refuses with `"Wait for the clips to finish, or press Stop, before clearing."` while `c.cancel != nil`; otherwise `store.Wipe()` and remove `scratch\`, nothing else. Bound on `App`: `ClearCache() error`.

- [ ] **Step 1: Failing tests:**
  `TestMakeOnlyRunsReadyClips` (Later and Made ids in the job list are ignored),
  `TestMakeUsesStoredFormatsAndQualityAndGivesScratchDir`,
  `TestFinishedClipsMoveToMadeWithoutAPage` (no event listener; after `run` returns the store has `made` rows with files, and the scratch folder is gone),
  `TestStoppedOrKilledClipStaysReadyAndDoesNotStartByItself` (cancel mid-run, reboot a new `Core` on the same dir: still `ready`, no run started),
  `TestStopThenMakeReusesTheSavedDownload` (first run cancelled after the download leaves `scratch\<id>\download.ok`; the second Make passes the same `ScratchDir`),
  `TestClearCacheRefusedWhileRunning`,
  `TestClearCacheWipesStoreAndScratchButNotSettingsOrOutput`.
- [ ] **Step 2:** `go test ./internal/appcore/` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** `go test ./... && go vet ./...` → PASS.
- [ ] **Step 5: Commit** `feat: record results on the Go side and resume unfinished clips`.

---

### Task 5: Lanes in `state.js`

**Files:**
- Modify: `cmd/vid2gif-app/frontend/state.js`, `cmd/vid2gif-app/frontend/state.test.js`

**Interfaces (pure, return new arrays):**
- Slips gain `lane` (`"ready" | "later" | "made"`), `filesMissing`, `resultKind`.
- `fromShelf(saved)` → slips (a made clip gets `status = {kind: resultKind, message: resultMessage, files, stage:"", pct:1, stagePct:-1}`).
- `inLane(slips, lane)`; `madeVisible(slips, showAll)` (newest first; 5 unless `showAll`).
- `markReady(slips, ids)` moves only `isReady` slips (valid, validated, not pending/failed/unavailable) to the end of Ready; `markAllReady(slips)` applies it to all Later; `moveToLater(slips, id)`.
- `makeAgain(slips, oldId, newId)` appends a copy of the made slip to Later with the new id, same setup, `status: null`, no result; the made slip is untouched.
- `applyStatus` also sets `lane:"made"` on `done|partial|failed|already`.
- `readyJobs`, `needsCheck`, `makeCount` only look at `lane === "ready"`; `addPlaceholders` puts new slips in `later`.
- `stampFor(slip)` → `{label, tone}`: `done|already` → `DONE`/`good`, `partial` → `PARTLY`/`bad`, `failed` → `FAILED`/`bad`.

- [ ] **Step 1: Failing tests** (one per function; key assertions): `readyJobs`, `needsCheck`, `makeCount` ignore later and made slips; `markAllReady` moves only valid Later slips in order and leaves one with no Start in Later; `moveToLater` changes only the lane; `applyStatus` `done` → lane made; `makeAgain` leaves the made slip untouched and appends the copy to Later; `madeVisible` is newest first and capped at 5 unless `showAll`; `fromShelf` round-trips a made clip with files and `filesMissing`; `stampFor` maps `already` to DONE and `partial` to PARTLY.
- [ ] **Step 2:** `node --test "cmd/vid2gif-app/frontend/*.test.js"` → FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4:** same command → PASS (existing tests updated only where they assumed one flat list).
- [ ] **Step 5: Commit** `feat: lanes in the page's shelf logic`.

---

### Task 6: Page: Ready to go and Later sections, saving

**Files:**
- Modify: `cmd/vid2gif-app/frontend/app.js`, `index.html`, `styles.css`, `dev-stub.js`

**Behaviour:**
- `index.html`: replace the single `<ol class="shelf">` with three `<section>`s (`sec-ready`, `sec-later`, `sec-made`), each with a heading (`Ready to go`, `Later`, `Made before`), a sticker count and its own list (the made one is built in Task 7). An empty Ready or Later shows one quiet line ("Nothing ready yet." / "New links land here."). Styling follows the `.superpowers` mockup `shelf-sections-v2`: `.sec-h` flex row, Bowlby heading 17px, `.slip.later { border-style: dashed }`. Later slips get a **Mark ready** button (disabled until ready-valid), and the Later header gets **Mark all ready**; Ready slips get **Move to Later**. The Make button counts only Ready.
- Saving: any change to a slip's times, `endKind`, formats or quality, a validation result, and every lane change calls `App.Save([slip])` straight away (no debounce); Remove calls `App.Remove`. The Make click first awaits every pending `Save` promise, since Go filters Make to stored `ready` clips.
- Boot: `slips = S.fromShelf(boot.shelf.clips)`; show `boot.shelf.notice` and `shelf:notice` messages via the existing `notice()`.
- `dev-stub.js`: `Boot()` returns a `shelf` with 2 ready, 2 later, 6 made (one failed, one `filesMissing`); stub `Save/Remove/ClearMade/ClearCache` to mutate an in-memory list.

- [ ] **Step 1:** Write the markup, styles and handlers.
- [ ] **Step 2:** `node --test "cmd/vid2gif-app/frontend/*.test.js"` → PASS.
- [ ] **Step 3: Verify in a browser** with the dev stub: `python -m http.server` in `cmd/vid2gif-app/frontend`, open `index.html`. Expected: three sections; Mark ready moves a valid slip up and is disabled for an invalid one; Mark all ready skips the slip with no Start; the Make count equals the Ready count; no console errors. If no browser tool is available, say so in the commit message and the final report.
- [ ] **Step 4: Commit** `feat: Ready to go and Later sections`.

---

### Task 7: Page: Made before log, Clear made, Clear cache

**Files:**
- Modify: `cmd/vid2gif-app/frontend/app.js`, `index.html`, `styles.css`, `dev-stub.js`

**Behaviour:**
- Made before renders `S.madeVisible(slips, showAll)` as the **return log** from the `.superpowers` mockup `made-before-shelf` option B: one `<button class="row">` per clip with a 44px thumb (or the `scene-none` art), full title, `FORMATS · Quality · day` meta (`toLocaleDateString` with weekday) and a rotated stamp from `S.stampFor`. A **Show more** button shows all and hides itself. Clicking a row opens the existing inline preview (reuse `paintPreview`) when files exist and `filesMissing` is false; otherwise the row expands to "File moved or deleted." Every expanded row has **Make again** (page-side: `S.makeAgain(slips, id, crypto.randomUUID())`, then `App.Save([copy])`) and **Remove**. Rows are real `<button>`s with `aria-expanded`.
- Made-before header has **Clear made** (`.btn.quiet`, no confirmation): `App.ClearMade()`, then drop `made` slips locally.
- Settings popover gets a "Clear cache" group below Helper files: the note "Forgets your shelf and history. Your clips folder is not touched." and a **Clear cache** button that asks inline ("Are you sure? This removes your ready and later clips too." with **Yes, clear** / **Cancel**, not `window.confirm`), disabled while `running`. On success `slips = []`, `previewId = null`, render. On the Go error, show it with `notice()`.

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `node --test "cmd/vid2gif-app/frontend/*.test.js"` → PASS.
- [ ] **Step 3: Verify in the browser with the dev stub:** the log shows 5 then all; the failed row has a red FAILED stamp; the `filesMissing` row says "File moved or deleted." and still offers Make again; Clear made leaves Ready and Later; Clear cache's inline confirm empties everything; Tab/Enter/Space work on rows and buttons.
- [ ] **Step 4: Commit** `feat: Made before log, Clear made and Clear cache`.

---

### Task 8: Docs, installer check, full verification

**Files:**
- Modify: `docs/development.md` (a short "Saved shelf" section: where `shelf.db` and `scratch\` live, how to reset, and a manual checklist), `installer/vid2gif.wxs` (line ~11 comment: mention `shelf.db` and `scratch\` in the `%LOCALAPPDATA%\vid2gif` removed on uninstall, after confirming that folder is what the installer removes)

- [ ] **Step 1:** Add the manual checklist to `docs/development.md`: close mid-download (reopen: clip in Ready, Make restarts it); close after the download finished (Make goes straight to encoding); kill the process during a run; type a time and close the window immediately; delete a made file and reopen; move the clips folder and reopen; damage `shelf.db` (notice shown, copy kept as `shelf.db.bad-*`); Clear made (Ready and Later stay, files stay); Clear cache (empty shelf at once, `settings.json` and clips untouched); add 30 links, mark 5 ready, Make makes only 5.
- [ ] **Step 2:** Verify, each must pass: `go build ./...`, `go vet ./...`, `go test ./...`, `node --test "cmd/vid2gif-app/frontend/*.test.js"`, and `go build -tags desktop,production -ldflags "-H windowsgui -s -w -X main.version=dev" -o dist/vid2gif-app.exe ./cmd/vid2gif-app` (confirm `dist/` is gitignored).
- [ ] **Step 3:** If a real window can be run, walk the checklist; otherwise say which items were not run.
- [ ] **Step 4: Commit** `docs: saved shelf notes and manual checklist`.
