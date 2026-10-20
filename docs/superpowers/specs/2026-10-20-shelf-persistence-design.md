# vid2gif desktop: a shelf that survives closing the app

Status: draft for review. Date: 2026-10-20.
Builds on branch `worktree-per-link-options` (each link has its own formats and quality, with the left panel as the default for new links). That branch is local and unmerged; this work assumes it lands first.

## Why

Today the shelf lives only in the page's memory. Close the exe and every link, every typed Start/Stop, and every result is gone. The person wants to come back the next day, look over yesterday's links, check their settings, press **Make clips**, and have only the clips they have signed off on be made. They do not want to work through 30 clips when 5 are ready.

So the shelf becomes saved work: a work queue (what is ready, what is not) with a history behind it (what was made).

## What the person sees

The right-hand side is three sections, top to bottom. The left panel is unchanged except for one hint line (already built on the per-link branch).

1. **Ready to go.** Slips the person has marked ready. The yellow Make button works on exactly these and counts exactly these. Each slip has a "Move to Later" link.
2. **Later.** Slips not yet marked ready, drawn with a dashed border (the app's existing "not active" look). Each has a "Mark ready" button, disabled until the slip has a valid Start. A "Mark all ready" button sits in the section header and only moves slips that are valid. **New links land here.**
3. **Made before.** The history, drawn as the **return log**: one thin line per clip with a small thumbnail, the full title, format and quality and day, and a rotated DONE / FAILED stamp. Chosen by the person over a "tapes on a shelf" alternative because it is more readable. Clicking a line opens its preview or folder, or **Make again**. Make again adds a **new** clip to Later (new ID) with the old one's setup (link, times, formats, quality) and no result; the history line stays as the record, so there is one row per attempt (my call). The stamps are DONE (green), FAILED (red), and PARTLY (red, same style as FAILED) when some formats were written and others failed; "already in your folder" shows as DONE (my call). It shows the latest 5 with a "Show more" link (my call on the number and the paging).

Section titles, the sticker counts, and the visuals follow the existing "Corner Video Shop" design system in `DESIGN.md` (ink-outlined paper, Bowlby One for the few shouted words, yellow reserved for Make and progress). The approved mockups are in the brainstorm folder under `.superpowers/` (not committed); the rules above are the contract.

### Clearing

- **Clear made** (button in the Made before header). Removes the made/failed history only. Ready to go and Later are untouched. Never deletes any file in the clips folder. No confirmation (my call: it only forgets history).
- **Clear cache** (in Settings, behind the gear). Deletes the whole SQLite store and all scratch downloads, and **empties the open window straight away**: Go wipes the store and resets its in-memory shelf, and the page shows the empty shelf at once, as on a first run. Later edits write into the fresh store. Asks "Are you sure?" once, because it throws away queued work (my call). It never touches the clips folder or `settings.json` (my call: the person said "delete the entire sqlite storage"). Disabled while a run is going (my call).
- Nothing expires on its own. The history grows until the person clears it.

### Interrupted work

If the app closes, crashes, or is stopped partway through a clip:

- The clip comes back in **Ready to go**, and **waits for the person to press Make**. It never starts by itself on launch. (Decided by the person.)
- If its download had **finished cleanly**, Make goes straight to encoding from the saved download. Any output file already written is skipped, as today. (Decided by the person.)
- If its download was **cut off partway**, it starts over. A half-written download is never trusted. (Decided by the person: "handling suggestions look right".)
- A clip that finishes moves to Made before and its saved download is deleted.

## Goals

- Everything on the shelf, in all three sections, comes back exactly as left, including typed-but-unvalidated times, per-link formats and quality, and playlist grouping.
- Closing the window mid-run still records what finished: results are written by the Go side as they happen, not by the page.
- Resume from a finished download after any interruption (see above).
- A damaged, locked, or too-new database never blocks the app from starting and never silently destroys the person's data.
- Careful engineering: a real, versioned, tested storage layer, since this is the first persistent state beyond `settings.json`.

## Non-goals

- Syncing across computers, export/import, or sharing a shelf.
- Automatic expiry or a retention setting.
- Auto-resuming on launch.
- Multiple app windows or processes sharing one store at once (must fail safe, see below, but is not a supported mode).
- Storing the made media itself in the database. It stays in the clips folder.
- A search/filter on history, editing history entries, or per-clip output folders.
- Telemetry of any kind.

## Anti-goals (a failure even if it technically works)

- A restart that makes a ready clip run without the person pressing Make.
- Make touching anything in Later or Made before.
- Clear made or Clear cache deleting a person's GIFs/videos, or Clear cache deleting `settings.json`.
- Losing typed times because the app was closed within a second of typing.
- A corrupt store turning into a blank window or a crash on launch.

## Constraints

- Windows desktop app, Wails v2, Go 1.26. CI and the build do not use cgo today. **SQLite driver (my call): `modernc.org/sqlite`, the pure-Go driver**, so the cgo-free build, the CI image, and the MSI pipeline do not change. `database/sql` with a single open connection and WAL mode.
- The tools (yt-dlp, ffmpeg, deno) and the clips folder logic are unchanged.
- Page code must keep the existing rules: remote text only via `textContent`/attributes, never `innerHTML`.
- Keep the repo's style: small packages under `internal/`, pure logic in `state.js` with `node --test`, Go tests next to the code.

## Design

### Storage

A new package `internal/shelf` owns the store. File: `shelf.db` in `dataRoot()` (`%LOCALAPPDATA%\vid2gif`, next to `settings.json`, so uninstall's single-folder cleanup still covers it; builder to confirm against `installer/vid2gif.wxs`). Scratch downloads live in `scratch\<clipId>\` in the same folder.

- Versioning: `PRAGMA user_version` plus an ordered list of migration functions in code. No migration library. Each migration runs in one transaction. Migration 1 creates the schema below.
- One table `clips` (the columns are the builder's to finalise; this is the required information):
  - identity and order: `id` (text, unique), `lane` (`ready` | `later` | `made`), `position` (order within the lane), `created_at`, `made_at`.
  - lookup result: `url`, `title`, `thumbnail_url`, `duration_sec`, `source` (playlist title or "Pasted links"), `unavailable`, `reason`, `lookup_failed`, `message`.
  - the person's input: `start_text`, `end_text`, `end_kind`, validated `start_sec`, `end_sec`, `time_msg`, `checked` marker, `formats`, `quality`.
  - result of the last run: `result_kind` (`done` | `partial` | `failed` | `already`), `result_message`, `files` (each with format, real path, size).
  - `interrupted` flag, set when a run ended without finishing that clip.
- IDs: **the in-memory counter (`s1`, `s2`, ...) restarts at 1 every launch and would collide with saved rows.** IDs become unique across launches (my call: a random 128-bit hex string; builder may choose another scheme). The page treats IDs as opaque.
- The Go side owns the store and is the source of truth after boot. The page keeps a working copy for rendering and writes changes through (see Flow).

### Flow

- **Boot** returns the shelf (three ordered lists) alongside settings. The page renders it before the first interaction. **Go writes a row the moment a link is added** (a pending placeholder in Later, with its URL), so a link is never lost if the app closes before its lookup answers. When the lookup answers, Go replaces that row in place with the resolved clip, or with several rows in the same position for a playlist (each keeps the playlist's title as `source`). A row still pending at boot reloads as a lookup-failed slip with **Try again** (my call).
- **Edits** (typing Start/Stop, switching to/from "length", changing a slip's formats/quality) are written through to Go per clip, debounced on typing (about 300 ms, my call) and flushed immediately on blur and on window close. Lane changes, removing a slip, and Make again write immediately.
- **Make** sends the ready list (as today). The Go run loop, not the page, records progress in the store: when a clip starts it is marked as making; when it ends, `result_*`, `files`, `made_at` are written and the lane becomes `made` in one transaction. If the app dies mid-run, on next boot any clip still marked as making is put back in `ready` with `interrupted` set. The page's local status events keep working as today.
- **Ready validity.** "Mark ready" requires a valid Start. Make keeps today's behaviour: ready slips whose typed times have not been validated yet (for example restored from disk, or edited just before pressing Make) are validated first, and then the valid ones are sent. A ready slip whose times turn out invalid stays in Ready showing the message and is skipped. The Make label counts valid ready slips plus ones Make will check first, as it does now. Make never reads Later or Made before.
- **Stop and interruptions.** A user **Stop** returns the current clip to Ready, as it does today. A crash or close during a run does the same on the next boot. In both cases the clip is flagged `interrupted`, and its row shows one plain line, "Stopped partway. Make carries on." The flag clears when the clip is next made. Whether it can skip the download is decided by the scratch folder and its marker, not by the flag (my call).
- **Made before** rows keep the output file list. At boot each file is checked for existence (cheaply, no hashing). A missing file shows "File moved or deleted" and no preview, but **Make again** still works. The local preview server must be told about every distinct output folder used by the saved history, not only the current one.

### Resume from a finished download

Today `clip.Run` downloads into a temp folder inside the clips folder and deletes it on every exit. The encodes write their part files and rename them into place, so finished outputs are already atomic and already skipped if present.

Change: split the two temp uses.

- The **download** goes to `scratch\<clipId>\`. When yt-dlp exits cleanly and the clip file is picked, write a small marker file there containing a fingerprint of everything that affects the download (link, Start, End, full-size setting, whether audio is needed). The scratch folder is kept on cancel/crash/failure and removed when the clip finishes (done, partly, or failed, so a history row never owns a scratch folder), when the clip is removed, or on Clear cache. Make again creates a new clip and so starts with a fresh download (my call; a failed clip's saved download is not reused).
- On the next run for the same clip: if the marker is present and its fingerprint matches, skip the download and encode from the saved file; otherwise delete the folder and download fresh. A folder with no marker is a cut-off download and is deleted.
- The **encode part files** keep going to a fresh temp folder in the clips folder, as today, because the final rename must stay on the same volume.
- Orphan sweep at boot (my call): delete any `scratch\<id>` folder with no matching clip row, and any `.vid2gif-tmp-*` folder from a dead run.
- Progress reporting: a resumed clip skips the download stage, so its progress bar starts at the encode stage. The progress package must not show a regression or a stuck bar; builder to handle.

### Failure handling for the store itself

- If the file cannot be opened, fails an integrity check, or has a `user_version` newer than the app knows: rename it aside to `shelf.db.bad-<timestamp>` (never delete), start with an empty store, and show one plain notice on the shelf ("Couldn't open your saved shelf. Starting fresh. A copy was kept."). Same for the case where the app was downgraded.
- If the store becomes unwritable at runtime (disk full, locked by another process), the app keeps working in memory, shows one notice, and retries on the next write. It must not crash or block Make.
- A second app process on the same store must fail safe (short busy timeout, then the in-memory behaviour above). Multi-window use is not supported.
- All store errors go to the debug log; the person never sees raw SQL or paths in messages (project rule: plain sentences).

### Settings and copy

- Settings gets one new row, **Clear cache**, with a short note ("Forgets your shelf and history. Your clips folder is not touched."), a confirmation, and disabled while a run is going.
- Strings stay in the app's voice: plain, warm, no tool names or jargon.

## Testing

- `internal/shelf`: round-trip of every field; migration from an empty database and from a database at each older version (start with a test that proves migration 1 is idempotent on reopen); lane moves and ordering; Clear made keeps Ready and Later; Clear cache removes the file and scratch; corrupt file and too-new version are moved aside and replaced; unwritable store does not panic.
- `internal/clip` / `runner`: with a fake downloader, interrupt between download and encode, then run again and assert the downloader is not called and the encodes are; assert a folder with no marker or a mismatched fingerprint re-downloads; assert finished clips delete their scratch folder.
- `appcore`: boot returns the saved shelf; an interrupted clip comes back in Ready with `interrupted` set and does not start by itself; results are recorded without a connected page.
- `state.js` tests: lane moves, Mark all ready only moves valid slips, Make count and label count only valid ready slips, Make again resets result but keeps setup, Show more paging.
- A manual checklist added to `MANUAL-TESTING.md`: close mid-download, close mid-encode, kill the process, delete a made file, damage `shelf.db`, Clear made, Clear cache.

## Left to the builder

Column names and types; the debounce number; the ID scheme; how the scratch fingerprint is hashed; the exact "Show more" page size; keyboard and screen-reader behaviour for the new sections and buttons (must meet the existing accessibility bar: names for every control, focus kept sensible after a slip moves lanes); whether the progress bar needs a new stage label for a resumed clip.
