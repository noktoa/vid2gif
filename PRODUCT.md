# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack
Desktop app in Go using Wails (the window is HTML/CSS/JS in a system webview, so the design platform is `web`). It imports the existing `internal/` packages (`clip`, `encode`, `naming`, `deps`) rather than calling the CLI. Windows only at first; Linux later with the same code. This was the builder's recommendation and the user delegated the choice ("least amount of work").

## Users
Non-technical people (friends and others the author shares it with, not a public audience) who want a GIF, WebP or video clip cut from online videos. They are not comfortable in a terminal. Primary situation: they have one or several video links, or a YouTube playlist, and know roughly when in each video the moment they want happens.

## Product Purpose
vid2gif cuts a segment out of an online video and saves it as a GIF, animated WebP, MP4, MOV, or any mix. The CLI already does this for one URL at a time. The desktop app makes it usable without a terminal: drop in video URLs or a playlist, enter start and end times per video, pick the format, quality and output folder, watch progress, and preview the result in the app. Success is a non-technical person making clips without ever seeing yt-dlp, ffmpeg or deno.

## Positioning
A friendly front end over a pinned, self-installing toolchain: no setup, no accounts, no hosting. Everything runs locally; the app fetches its own pinned yt-dlp, ffmpeg and deno on first launch (what `deps install` does today).

## Operating Context
- Windows desktop, local files, output folder chosen by the user and remembered.
- Input: pasted or dropped video URLs, or a YouTube playlist link. A playlist behaves like a dropped zipped folder: it expands into one row per video. Each row takes its own start and end time.
- Times are `SS`, `MM:SS` or `HH:MM:SS`; start is required, end is optional (blank means to the end of the video). The CLI rule applies: end must be after start.
- Formats: gif, webp, mp4, mov (any mix). Quality: low, medium, high. The format and quality apply to the whole batch.
- Each clip is a download plus an encode; clips run one after another, each with its own progress, and one failing (private or removed video) does not stop the rest.
- A finished clip can be previewed inside the app, and its folder opened.

## Capabilities and Constraints
- Existing CLI behaviour to preserve: existing files are skipped unless overwrite is on; no sound in GIF/WebP; GIF/WebP never scaled up; temp files live in a `.vid2gif-tmp-*` folder inside the output folder.
- Needs adding to the shared Go code: playlist expansion (a playlist listing from yt-dlp) and progress reporting. Neither exists today.
- First-launch tool download is about 250 MB and needs a visible, friendly progress state.
- The user can pick a colour theme in a Settings menu (default cool grey). The setting is remembered.
- Out of the first version (builder's call, not confirmed by the user): per-row format override, parallel clips, drag-reorder of rows, a live preview of the start frame while setting times, Linux and macOS.

## Brand Commitments
Name: vid2gif. No other voice, logo or identity commitments yet.

## Evidence on Hand
`README.md` and `docs/superpowers/specs/2026-10-17-vid2gif-design.md` (CLI behaviour). No screens, logo, or user research exist. Nothing here is from real user testing.

## Product Principles
- A non-technical person should never see a tool name, a flag, or a command.
- The list of videos is the main thing; settings stay out of the way until needed.
- Failure is per-row and plain-language, and never loses the rest of the batch.
- Local-first: nothing to host, sign into or upload.
- The CLI and the app share one engine.

## Accessibility & Inclusion
No product-specific standard established. Default to keyboard operability and readable contrast for a general audience.
