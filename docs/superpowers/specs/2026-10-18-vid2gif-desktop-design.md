# vid2gif desktop app: design

Date: 2026-10-18
Status: draft for review

Builds on `2026-10-17-vid2gif-design.md` (the CLI). Read that first; this document only covers what the app adds or changes.

## Intent

The CLI works, but a terminal is a wall for non-technical people. The author wants to share vid2gif with friends who will never type a command. The app lets them drop in video links (or a YouTube playlist), type a start and stop time per video, press one button, and get GIF, WebP, MP4 or MOV clips, with progress and an in-app preview.

It must run entirely on the user's machine: no server, no accounts, nothing to host. The author's phrase for the build priority: "the least amount of work".

## Goals

- A non-technical person makes clips without ever seeing a tool name, a flag or a command.
- One engine: the CLI and the app share the same Go packages, so a fix in one reaches both.
- A playlist behaves like an unzipped folder: it becomes a list of videos, and each video gets its own start and stop.
- Progress and failures are visible per video. One video failing never stops the rest.
- The user can preview a finished clip inside the app and open its folder.
- The look is a deliberate world (see Visual design), not a generic converter app.

## Non-goals (version one)

These are the builder's call (the user did not rule on them; they can be added later):

- Choosing a different format per video (format and quality apply to the whole batch).
- Running clips in parallel (they run one after another).
- Reordering slips by dragging.
- A live preview of the start frame while typing times.
- Linux and macOS builds. The app is Windows (amd64) only; Wails can add Linux later from the same code.
- Auto-update, accounts, analytics, or any network service of our own.

## Anti-goals (would make it a failure even if it works)

- It looks like a developer tool (terminal vibes, monospace everywhere, yt-dlp or ffmpeg or "CRF" showing in the UI). Stated by the user.
- It looks like generic SaaS (purple gradients, cards-in-cards, dashboard feel). Stated by the user.
- Warm cream or off-white backgrounds. The user said people read them as AI-made. Stated by the user.
- A settings-heavy screen. A non-technical person should see only what they need.

## Constraints

- Windows 11 desktop, built with Go and Wails (the window is HTML/CSS/JS in the system WebView2). Chosen by the builder; the user delegated the stack ("least amount of work").
- The app imports the existing `internal/` packages (`clip`, `encode`, `naming`, `deps`). The CLI keeps working unchanged in behaviour.
- All three tools stay pinned exactly as in the CLI spec. The app never uses a different yt-dlp, ffmpeg or deno than the pinned one.
- It works offline for everything except downloading videos and the first-launch tool download.
- Audience: friends the author shares an .exe or an installer with. Not a public release.
- The app must not assume where it is installed. The helper tools live in a `bin` folder inside whatever folder the app was installed to (the user's requirement); an all-users install has one shared copy, a per-user install has its own. Settings, logs and the web view's data stay per user under `%LOCALAPPDATA%\vid2gif`.

## Decisions the user made

- Native desktop app, not a hosted service.
- Input is a list of video URLs or a YouTube playlist; a playlist is just a collection of video URLs. Start and stop times are entered per video.
- A progress bar, a place to choose the output folder, and a preview of the made GIF/WebP/video.
- Stay in Go and share the engine, rather than rewriting in C# (chosen for least work).
- Layout B (below).
- Colourful backgrounds the user can switch in a Settings menu. The default is cool grey.
- The helper tools folder is relative to the install location, with a single shared place for an all-users install, the same arrangement as a per-user install.

## Layout (chosen: "B, counter on the left")

A runnable HTML mock of both candidate layouts is in `2026-10-18-vid2gif-desktop-mockup.html` (layout B is the second one). It is a throwaway reference for look and structure, not shipping code. It loads fonts from Google Fonts; the real app must bundle them.

Window: a left **counter** column (about 290 px) and a right **shelf**. This is the wide layout; see Responsive behaviour below for smaller windows. The window can be resized freely down to a minimum of about 480 x 560 (builder's call).

### Responsive behaviour (required; the user wants it to work properly at about 600 px wide, half a screen)

- **Wide, about 900 px and up:** the layout above. Extra width goes to the shelf.
- **Medium, about 640 to 900 px:** the counter narrows (about 240 px) and each slip tightens: the Start and Stop boxes and the status wrap under the title instead of sitting beside it.
- **Narrow, under about 640 px (must work well at 600):** the counter moves to the top of the window, above the shelf, as a compact block (drop area, format chips, quality, save folder). The yellow Make button is pinned to the bottom edge so it is always reachable. Each slip stacks into a single column: stub with thumbnail and length, title, Start and Stop, then status. The inline preview takes the slip's full width.
- Every theme is checked at all three sizes. No horizontal scrolling at any width down to the minimum. Touch-sized controls are not required, but click targets stay at least 32 px high.
- The mock's left column is a fixed width, so the mock does not show the narrow layouts. The builder must verify them in the real app.

**Counter (always visible):**
1. Drop area: "Drop video links or a playlist here", plus a paste field and an Add button. Dropping text, dragging a link in, or pasting all work.
2. "Make them as": toggle chips GIF, WebP, MP4, MOV (one or more; at least one stays on). Default GIF.
3. "Quality": Small file / Balanced / Sharp, mapping to `low` / `medium` / `high`. Default Balanced.
4. "Save to": the folder path and a Change button (native folder picker). Default `Videos\Clips` in the user's profile (builder's call).
5. The yellow primary button, "Make N clips" where N counts the videos that are ready. While a run is going it becomes "Stop" and cancels the current work (same behaviour as Ctrl+C in the CLI: cancel the child processes and delete the temp folder).

**Shelf (right):** a header line naming the source (playlist title, or "Pasted links"), a count sticker, then one **slip** per video.

A slip shows: thumbnail and length on the left stub; the video title and where it came from; a Start box and a Stop box; and a status area on the right. Times accept `SS`, `MM:SS`, `HH:MM:SS` with decimals on the last part, exactly as the CLI. Start is required. Stop is optional and means "to the end of the video"; the Stop box shows "end" as its placeholder (builder's call). Stop must be after Start.

**Slip states** (all shown in the mock):
- Ready: times set, no mark (the mock shows "Next up" while a run is going).
- Needs a start time: the Start box has a dashed outline and the note "Add a start time to include this one." Such slips are left out of the run and not counted in the button.
- Making: a yellow progress bar with a stage label (see Progress accuracy), for example "Getting the clip 63%".
- Done: a green DONE stamp and the formats made ("GIF + MP4"). Clicking it opens the preview inside the slip.
- Could not get the video: a red NOT FOUND stamp, a plain sentence ("Couldn't get this video. It may be private or removed."), and a Remove link. Other failures use the same pattern with their own plain sentence (never show raw tool output). The known ones:
  - Link that cannot be read at all (not a video page, lookup failed): the placeholder slip becomes a failed slip with a plain sentence and Remove.
  - No internet: "Couldn't reach the internet. Check your connection and try again."
  - Output folder unusable: "Couldn't save to that folder. Pick another one."
  - The site stopped working with the pinned yt-dlp (extraction errors, sign-in or bot checks, HTTP 403): "This site isn't working with the app right now. An update may fix it." This is the most likely long-term failure and it is not the user's fault.
  - Anything else: "Something went wrong with this one." followed by a hint that depends on debug mode: off, "Turn on troubleshooting in Settings and try again to save details."; on, "Details were saved to the log." with a Show log link on the slip.
  - Helper tools missing or the wrong version after setup (deleted, quarantined): the app returns to the first-launch screen instead of failing each slip.
- Already made (builder's call): when a file with the same name already exists for every chosen format (same video title, same Start and Stop, same quality; a different quality is a new file, see Engine changes item 9), the slip says "Already in your folder" and nothing is written. This mirrors the CLI's skip-unless-overwrite rule; to redo it the user deletes the file.

**Preview (inside the finished slip):** a player (the clip, looping), format tabs for the formats that were made, the length, file size and width, and Open folder and Close. The webview plays GIF, WebP and MP4. MOV is not reliably playable in WebView2, so a MOV-only result shows no player and just Open folder (builder's call).

**The clip under the eye:** exactly one slip at a time has the magenta ring: the one being previewed, or else the one currently being made. Magenta is used for nothing else.

**Settings menu:** a gear in the corner of the shelf header opens a small menu with a row of colour swatches. Choosing one applies immediately and is remembered. It shows the helper-files folder (read-only) with an Open button. It also has "Download size": "Faster (recommended)" or "Full size (slower, slightly sharper)" (the `FullSource` setting; default Faster). It also has a **Troubleshooting** switch (debug mode, off by default) with a short note ("Saves a log of what the app does, including the video links you use, so a problem can be diagnosed.") and an Open logs folder button. Colour, download size and troubleshooting are the only settings in it for now.

**First launch:** if the pinned tools are missing, the window shows a friendly full-window screen ("Getting ready. This only happens once.") with one progress bar for the roughly 250 MB download, then drops into the normal window. Download or checksum failure shows a plain message and a Try again button. If the app cannot write to its `bin` folder (an all-users install under `Program Files`, used by a standard user), the screen instead says "The helper files couldn't be set up for your account. Ask an administrator to run Repair for vid2gif in Windows' installed apps, or reinstall it for just you." The mock does not show this screen; its look follows the same style (builder's call).

## Installer

The user asked for an MSI installer as an option (and said the install path must not be assumed). Decisions:
- **MSI built with WiX Toolset v4 or later** (builder's call on the exact version), using the Advanced UI so the user chooses **just me** (default, no admin rights, installs under `%LOCALAPPDATA%\Programs\vid2gif`) or **all users** (asks for admin, installs under `Program Files`), and can change the install folder. Start Menu shortcut, an entry in Windows' installed-apps list, a clean uninstall of what it installed, and upgrades in place when a newer version is installed.
- **The installer package does not contain the tools** (user's choice), so the MSI is about the size of the app (roughly 15 MB) and we do not redistribute the GPL ffmpeg build ourselves. Instead, after copying files the installer runs `vid2gif-app.exe --install-tools` (elevated for an all-users install) to download the pinned tools into `<install folder>\bin`, on install, upgrade and Repair. If that step fails (offline, proxy), the install still succeeds and the first launch fetches them wherever the user can write. Because only administrators can write under `Program Files`, other users cannot swap the tools in an all-users install.
- The plain `vid2gif-app.exe` is still published for people who do not want an installer.
- **Uninstalling is a clean removal (the user's requirement).** It removes the program, the shortcut, the Start Menu entry, the `bin` folder with the tools, and the whole data folder `%LOCALAPPDATA%\vid2gif` (settings, logs, the web view's data). The install folder itself is removed only when it is empty afterwards, so a folder the user already had (for example `D:\Apps`) is never wiped.
- **Never removed:** the clips the user made, and the output folder they chose. Temp folders left in an output folder by a hard crash (`.vid2gif-tmp-*`) are also left alone, because the installer does not know which folders were used (the README says they are safe to delete).
- Limit, stated in the README: an all-users install removes the data of the user who runs the uninstall; other Windows users' `%LOCALAPPDATA%\vid2gif` folders stay until they delete them.
- The MSI product version comes from the release tag (`v0.1.0` becomes `0.1.0`).
- The MSI is unsigned, so Windows SmartScreen will warn on first run. Say so in the README. Code signing is out of scope.
- WebView2: Windows 11 includes it. If it is missing, the app shows Wails' own prompt to install it (builder verifies this on a machine without the runtime, or says so if it cannot).

## Visual design

The look is the **Corner Video Shop** world: each video is a rental slip with a perforated stub, stamps mark state, tabular time figures line up in columns. The reasoning, the tokens and the rejected alternatives are in `PRODUCT.md` and `.impeccable/surfaces/frontend-index-html.md` (the direction contract). `DESIGN.md` is written when the app is built, from the built result.

Fixed in every theme: ink `#1E1B24`; sticker yellow `#FFD60A` for the primary button and progress; magenta `#E6007E` for the one ring; done green `#00805A` (text) / `#00A66E` (fills); error `#C23B22`.

Themes (background, slip paper, line, title bar):

| Theme | Ground | Slip | Line | Title bar |
|---|---|---|---|---|
| Cool grey (default) | `#E4E8ED` | `#F8FAFC` | `#CBD1D9` | `#DCE1E8` |
| Mint | `#CDEBE4` | `#F7FCFB` | `#9BC7BD` | `#B8DDD5` |
| Lilac | `#E4DEF6` | `#FBF9FF` | `#BDB3DB` | `#D4CCEB` |

The three themes in the table were shown to the user. Every theme must keep text at least 4.5:1 and the yellow button and magenta ring clearly visible; the builder checks this per theme.

Type: Bowlby One for the logo and stamps, Atkinson Hyperlegible for everything else, tabular figures (`tabular-nums`) for times and sizes; if Atkinson's digits turn out not to be tabular, Barlow Semi Condensed for those only. The app must bundle these fonts (no network at runtime).

Rules the user or the style rejects: no warm cream grounds, no gradient text, no eyebrow labels above headings, no cards inside cards, no modal for the preview.

## Engine changes

The CLI today shells out and streams yt-dlp and ffmpeg output to a terminal writer. The app needs structured information instead. These are additions to the shared packages; the CLI's visible behaviour stays the same, with one exception (item 7: GIF/WebP downloads get faster).

1. **Playlist expansion.** Given a URL, decide whether it is a single video or a playlist and return, per video, the id/URL, title, duration and thumbnail URL, using yt-dlp's metadata-only listing (no download). A pasted single URL becomes one slip. Unavailable entries become slips already in the "could not get" state.
2. **Progress reporting.** `clip.Pipeline.Run` reports stages and fractions through an optional callback (see Progress accuracy for what can and cannot be measured). The CLI ignores it.
3. **Cancellation.** Already supported by the context passed to child processes; the app's Stop button uses it.
4. **Result facts for the preview.** After a clip, report the written file paths with size, plus length and width, so the preview can show them.
5. **Settings file.** Theme, output folder, formats, quality and download size, stored as JSON under the user's profile (`%LOCALAPPDATA%\vid2gif\settings.json`; everything the app creates lives under the one folder `%LOCALAPPDATA%\vid2gif`, so removal is simple; builder's call). A missing or corrupt file falls back to defaults.
6. **Helper-files location (the user's requirement: relative to the install location).** The pinned tools live in a `bin` folder inside the install folder, next to `vid2gif-app.exe`; `VID2GIF_BIN` overrides it (same rule as the CLI, shared code). An all-users install therefore has one shared set of tools and a per-user install has its own. The folder is filled either by the installer (a headless `vid2gif-app.exe --install-tools` step, see Installer) or by the first-launch screen when the user can write there (a per-user install, or the plain .exe in a writable folder). `deps.json` is embedded in the binary. `deps.Install` and the resolver are reused; the CLI keeps `bin/` next to `deps.json`.
7. **Smaller source for GIF and WebP (the user approved it as the recommended default; changes the CLI too).** The user also wants a way to turn it off: a `FullSource` setting in the app (Settings menu, "Download size": "Faster (recommended)" or "Full size (slower, slightly sharper)") and a `--full-source` flag in the CLI. By default, when a run asks only for `gif` and/or `webp`, download the smallest video format whose width is at least the quality preset's maximum width (320, 480 or 720), and the best available when none is that wide. With `FullSource` on, or when any of mp4/mov is requested, the download is the best available as today. The output must have the same dimensions as today (`min(preset width, source width)`). Update the CLI spec and README, which currently say `-f "bv*/b"`. Measure the speed-up before and after on the same clip and record it in the commit message.
8. **Debug mode (the user asked for this; off by default).** A `DebugMode` setting, switchable in Settings without restarting. While it is on, the app writes log files to `%LOCALAPPDATA%\vid2gif\logs` (inside the data root, so uninstall removes them). While it is off, no log file is created. What a log holds: app and tool versions, OS version, the settings in use (no secrets exist), each lookup and clip request, the exact command lines run, the full stderr of yt-dlp and ffmpeg, the clip events, and the original error behind every plain-language message. Video links appear in the log, and the Settings note says so; query strings of signed media URLs (for example `googlevideo.com` links with `ip=` and `expire=`) are redacted. One file per app run; the newest 5 files are kept (builder's call on the number). The CLI does not get this feature.
9. **Quality is part of the default file name (the user asked for this; changes the CLI too).** Making the same clip again at a different quality must create a new file, not be skipped as a duplicate and not overwrite the earlier one, because the user may be comparing qualities. The default name becomes `<title>_<start>-<end>` for the default `medium` quality (so existing names do not change) and `<title>_<start>-<end>_low` or `_high` otherwise. A name given with `--name` is used exactly as typed. The download-size setting is not part of the name: it does not change the output's dimensions. Same title, same times and same quality is the only case treated as already made.

## What the user must be able to do (acceptance, plain words)

- Paste or drop one link, several links, or a playlist, and see one slip per video with its title, thumbnail and length.
- Type times in the three accepted formats; see a plain message when a time is invalid or Stop is not after Start.
- Press the yellow button and watch each slip go from waiting to making to done in order, while a bad video shows its own error and the rest continue.
- Press Stop partway; the current clip's temp files are cleaned up.
- Click a done slip, see the clip play in place, switch between the formats made, and open the folder it was saved in.
- Switch colour themes from Settings and find the same theme after restarting.
- On a machine with none of the tools, start the app, wait through one first-launch screen, and then make a clip with no other setup.

## Progress accuracy

The user's rule: a progress bar that isn't accurate is worthless. These are measurements, taken on the user's PC on 2026-10-19 with a 10-second clip of a 4K YouTube video (treat them as ballpark; they vary by machine, network and source. The encode figures also come from one source clip, the same 10 s repeated to 60 s on one machine, so other content could vary more):

| Phase | Time | Real signal |
|---|---|---|
| Finding the video | about 3 s | none |
| Getting the clip | about 7 s | yt-dlp prints no percentages for a sectioned download, only a final "100%". The ffmpeg process it starts prints `time=HH:MM:SS.xx` on stderr; divided by the clip length it is accurate and roughly linear (slow at the very start, a jump at the very end) |
| Making a GIF | about 2 s (9 s for a 60 s clip) | none useful: 6 samples for the 60 s clip, up to 64 points away from a straight line (the palette pass reads everything first) |
| Making a WebP | about 1 s | none useful |
| Making an MP4 or MOV | about 7 s | real (`-progress` `out_time_us`): on a 60 s clip, 73 to 82 samples per run, within about 3 percentage points of a straight line, but one small step backwards (about 1 s of the clip) in 3 of 4 runs, so the fraction must be clamped to never decrease |

Decision (user): **one bar plus a stage label.**
- The bar only moves when real work finishes. It never sits at 100% while work continues, never goes backwards, and reaches 100% only when the clip is done.
- Labels: "Finding the video…", "Getting the clip 63%", "Making GIF…", "Making MP4 40%". A percentage appears only on the two measured stages (getting the clip, making MP4/MOV). Unmeasured stages show the label and a gentle pulse. No time estimate is ever shown.
- How far each stage moves the bar comes from a cost table of measured seconds of work (find: about 3 s fixed; getting the clip, GIF, WebP, MP4/MOV: a per-second-of-clip cost), kept in one place in `internal/runner` with the date, machine and sample size in a comment. Re-measure it after the smaller-source change (engine item 7), because that shrinks the largest stage.
- The clip length is Stop minus Start, or the video's duration minus Start when Stop is blank. When it is unknown (a live video), the bar is indeterminate for the whole clip and the label still changes with the stage.
- With reduced motion, the pulse is replaced by a static striped fill.

## Feel (snappy)

The user wants the app to feel snappy. A throwaway Wails test (an empty window with the layout B mock, fake progress events at 25 per second) was run on the user's PC and they said it looked good; no numbers were recorded. That settles Wails as the stack. Requirements:

- The window appears in about a second on the user's PC, and the page never freezes while clips are made.
- A dropped or pasted link shows a slip at once, with a placeholder title and thumbnail, then fills in as the lookup returns.
- Progress updates are throttled (about 10 a second per clip is plenty) so the page is not redrawn needlessly.
- The title comes from the playlist or link lookup, so the app does not start yt-dlp a second time just to ask for the title (the CLI does; the shared code should accept a title that is already known).
- No heavy frontend framework. Plain HTML, CSS and JS, as in the mock.

## Testing

Unit tests for the new Go pieces (playlist-listing parser, progress parser, settings load/save with a corrupt file) in the same style as the existing tests, which need no network or tools. The existing integration test keeps covering the tool installer and encodes. UI is checked by hand against the mock, per theme.

## Small calls made after the builder check (the designer's calls, not the user's)

- **A video link that also carries a playlist** (`watch?v=…&list=…`) is treated as that one video, matching the CLI's `--no-playlist`. Only a pure playlist link expands into a list.
- **Stop cancels the whole run.** The current clip is cleaned up and goes back to ready; the clips not yet started stay ready; clips already done stay done.
- **Partly failed slip.** If some formats were made and one failed, the slip shows DONE for what exists plus a plain note ("The MP4 couldn't be made."). If nothing was made, it shows the error state.
- **Serving the preview.** The page plays files from the chosen output folder through Wails' asset handling or a small local file handler limited to files the app just wrote. The mechanism is the builder's choice; the limit to the app's own outputs is not.

## Left to the builder

Project layout inside the repo (for example `cmd/vid2gif-app` plus a `frontend/` folder), the frontend approach (plain HTML/CSS/JS is enough and matches the mock; no framework required), exact event and API shapes between Go and the page, copy for less common errors, and the packaging of the single .exe (extend `release.ps1`).
