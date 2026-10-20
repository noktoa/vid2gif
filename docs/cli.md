# Command line

```
vid2gif clip URL --start T [flags]
```

The URL always comes first, then the flags. Run `vid2gif help` to see the flags in the terminal.

## Flags

| Flag | Default | What it does |
|---|---|---|
| `--start T` | required | Where the clip begins. |
| `--end T` | the end of the video | Where the clip ends. Must be after the start. |
| `--format LIST` | `gif` | Any of `gif`, `webp`, `mp4`, `mov`, separated by commas. |
| `--out DIR` | `./gifs` | Where the files are written. Created if it doesn't exist. |
| `--name BASE` | `<video title>_<start>-<end>` | File name without the extension; each format adds its own. Used exactly as typed. See [Default names](#default-names). |
| `--quality Q` | `medium` | `low`, `medium` or `high`. See [Quality](#quality). |
| `--overwrite` | off | Replace files that already exist. |
| `--full-source` | off | GIF and WebP only: download the best video instead of the smallest one wide enough for the quality. See below. |
| `--no-pin` | off | Skip the tool version check. The tools folder is still searched first, then your PATH. deno is optional here. |
| `--no-progress` | off | Don't draw the progress bar. |

Times are `SS`, `MM:SS` or `HH:MM:SS`. Only the last part can have decimals (`1:23.5`).

Without `--end` the clip runs to the end of the video. On a long video, give an end time unless you want everything after the start; a GIF of that much footage gets very large.

### Default names

The default is `<video title>_<start>-<end>`, in seconds. With no `--end` the end part is the word `end` (`title_83-end`). At `--quality low` or `high` the name ends in `_low` or `_high` (`title_83-88_high`), so the same clip at another quality is a new file, not a skip. `medium` adds nothing.

### `--full-source`

Normally vid2gif downloads the smallest video at least as wide as the quality needs (320, 480 or 720 pixels). That's a smaller download and gives a GIF or WebP of the same size. `--full-source` downloads the best video instead. MP4 and MOV always use the best video.

## Existing files

If an output file already exists, vid2gif skips it, tells you, and carries on with the other formats. If every requested file exists, it downloads no video (it still asks yt-dlp for the title unless you gave `--name`). Use `--overwrite` to replace them.

## Quality

| Preset | GIF/WebP frame rate | GIF/WebP max width | MP4/MOV compression (CRF) |
|---|---|---|---|
| `low` | 10 fps | 320 px | 28 |
| `medium` | 15 fps | 480 px | 23 |
| `high` | 24 fps | 720 px | 18 |

- GIF and WebP are resampled to the preset's frame rate (a lower-rate source gets frames repeated) but play at the same speed. They are never scaled up. They have no sound.
- MP4 and MOV keep the original frame rate and size (rounded down to even pixel counts), and keep the audio.

## Progress

In a terminal, `clip` shows one line that updates in place, with the same wording and numbers as the desktop app:

```
Getting the clip 63%    [█████████░░░░░░░░░░░░░░░]  41%
```

- The percentage is for the whole clip. It never reaches 100% before the clip is done, never goes backwards, and there is no time estimate.
- GIF and WebP encoding can't be measured, so during those steps a block moves along the bar instead of a number. Without `--end` the clip's length is unknown, so the bar only shows activity.
- Outside Windows Terminal on Windows, or without a UTF-8 locale elsewhere, the bar uses plain characters (`[#####-----]`).
- While the bar is shown, yt-dlp's own messages are held back; if the run fails they print before the error. The `wrote …`, `skipped …` and `failed …` lines print as usual.
- When output isn't a terminal (redirected, piped, or sent to `NUL`), or with `--no-progress`, there is no bar.

## How a run works

1. Checks your times and flags first, so a typo doesn't cost a download.
2. Finds yt-dlp, ffmpeg and deno and checks their versions against the pins.
3. Asks yt-dlp for the title (unless you gave `--name`) and works out which files to write.
4. Downloads only the requested section, once, however many formats you want.
5. Encodes each format with ffmpeg and moves the result into the output folder.
6. Cleans up its temporary `.vid2gif-tmp-*` folder inside the output folder, including on Ctrl+C. If the process is killed hard, you can delete that folder.
