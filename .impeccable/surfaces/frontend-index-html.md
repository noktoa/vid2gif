---
version: 1
slug: "frontend-index-html"
primary_target: "frontend/index.html"
related_targets: []
---

# Main window

Mode: Operate. A non-technical person turns a list of online videos into GIF/WebP/MP4/MOV clips. Occasional use at a desk. Task: add videos or a playlist, set Start and Stop per video, choose format, quality and folder, make the clips, watch progress, preview results.

Audience, job, constraints: friends the author shares the app with; Windows-only Wails webview; light themes only (daytime desk use, normal ambient light). Anti-goals: must not read as a developer tool or generic SaaS. No tool names or flags in the UI.

Mockup scope: two competing layouts for this one window, throwaway, shown in the brainstorming visual companion. Not shipped code.

## Direction contract

THESIS: Every video is a tape on the rental shelf, handed over on a slip with an in-point and an out-point, stamped when it is done. Refuses the converter-app arrangement of a table of filenames over a settings sidebar.

OWN-WORLD: Switchable colour themes (user setting), default cool grey: ground #E4E8ED, slip #F8FAFC, line #CBD1D9; alternates mint #CDEBE4, lilac #E4DEF6, sky blue. Warm cream grounds are refused. Ink #1E1B24 and the accents below do not change per theme. one hot magenta #E6007E reserved for the single row under the eye, sticker yellow #FFD60A for the primary action and "in progress", rental-stamp green #00A66E for done. Rental-slip card stock rows with a perforated stub edge, round NEW RELEASE sticker dots for counts and status, rubber-stamp DONE and RETURNED marks. Times in tabular figures in fixed columns. A face with shop-signage character for titles, a plain humanist sans for controls; neither from the banned default list.

STORY: The visitor understands in seconds that each slip is one video and that Start and Stop are the only things they have to fill in. They believe nothing can go wrong in a way they cannot read, and they press the yellow button.

FIRST VIEWPORT: Layout B (chosen). A left counter column holds the drop area, format chips, quality, save folder and the yellow Make clips button, always visible. The right side is the shelf of rental slips, one per video: thumbnail on the stub, title, Start and Stop boxes in tabular figures, a status mark on the right. A finished slip opens its preview inline, inside the slip, never a modal. A gear opens a Settings menu with colour swatches.

FORM: Assigned candidate 4 of 7 (Corner Video Shop), seed key d1d169c1. Raises: tabular time columns (ikeda-datamatics), one reserved ring colour for the clip under the eye (notation-diagram-systems-star-atlas).

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
