---
name: vid2gif
description: A corner video shop on your desktop. Each video is a rental slip with an in-point and an out-point, stamped when it is done.
colors:
  ink: "#1E1B24"
  ink-2: "#4A4655"
  hint: "#6A6676"
  paper: "#FFFFFF"
  ground-grey: "#E4E8ED"
  slip-grey: "#F8FAFC"
  line-grey: "#CBD1D9"
  titlebar-grey: "#DCE1E8"
  ground-mint: "#CDEBE4"
  slip-mint: "#F7FCFB"
  line-mint: "#9BC7BD"
  titlebar-mint: "#B8DDD5"
  ground-lilac: "#E4DEF6"
  slip-lilac: "#FBF9FF"
  line-lilac: "#BDB3DB"
  titlebar-lilac: "#D4CCEB"
  sticker-yellow: "#FFD60A"
  sticker-yellow-hover: "#FFE04D"
  eye-magenta: "#E6007E"
  stamp-green: "#00805A"
  alert-red: "#C23B22"
typography:
  sign:
    fontFamily: "Bowlby One, Arial Black, sans-serif"
    fontSize: "18px"
    fontWeight: 400
    lineHeight: 1
    letterSpacing: "0.02em"
  logo:
    fontFamily: "Bowlby One, Arial Black, sans-serif"
    fontSize: "26px"
    fontWeight: 400
    lineHeight: 1
    letterSpacing: "0.01em"
  headline:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, sans-serif"
    fontSize: "22px"
    fontWeight: 700
    lineHeight: 1.25
  title:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, sans-serif"
    fontSize: "15px"
    fontWeight: 700
    lineHeight: 1.25
  body:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1.4
  label:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, sans-serif"
    fontSize: "12px"
    fontWeight: 700
    lineHeight: 1.4
  time:
    fontFamily: "Atkinson Hyperlegible, Segoe UI, sans-serif"
    fontSize: "19px"
    fontWeight: 700
    lineHeight: 1
    letterSpacing: "0.02em"
    fontFeature: "tnum"
  stamp:
    fontFamily: "Bowlby One, Arial Black, sans-serif"
    fontSize: "15px"
    fontWeight: 400
    lineHeight: 1
    letterSpacing: "0.06em"
rounded:
  sm: "6px"
  md: "8px"
  lg: "10px"
  xl: "12px"
  pill: "99px"
spacing:
  xs: "6px"
  sm: "8px"
  md: "14px"
  lg: "18px"
  xl: "28px"
components:
  button-make:
    backgroundColor: "{colors.sticker-yellow}"
    textColor: "{colors.ink}"
    typography: "{typography.sign}"
    rounded: "{rounded.xl}"
    height: "50px"
    padding: "13px 20px 12px"
  button-make-hover:
    backgroundColor: "{colors.sticker-yellow-hover}"
  button-make-disabled:
    backgroundColor: "{colors.slip-grey}"
    textColor: "{colors.ink-2}"
  button:
    backgroundColor: "{colors.slip-grey}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    height: "36px"
    padding: "6px 12px"
  button-hover:
    backgroundColor: "{colors.paper}"
  chip:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.pill}"
    height: "32px"
    padding: "4px 14px"
  chip-pressed:
    backgroundColor: "{colors.ink}"
    textColor: "{colors.slip-grey}"
  field:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.md}"
    height: "36px"
    padding: "6px 10px"
  slip:
    backgroundColor: "{colors.slip-grey}"
    textColor: "{colors.ink}"
    rounded: "{rounded.lg}"
  sticker:
    backgroundColor: "{colors.paper}"
    textColor: "{colors.ink}"
    rounded: "{rounded.pill}"
    height: "30px"
    padding: "0 8px"
  stamp-done:
    textColor: "{colors.stamp-green}"
    typography: "{typography.stamp}"
    rounded: "{rounded.sm}"
    padding: "5px 9px 4px"
  stamp-bad:
    textColor: "{colors.alert-red}"
    typography: "{typography.stamp}"
    rounded: "{rounded.sm}"
---

# Design System: vid2gif

## Overview

**Creative North Star: "The Corner Video Shop"**

Every video is a tape on the rental shelf, handed over on a slip with an in-point and an out-point, then stamped when it is done. The window is a counter on the left (drop, format, quality, folder, the yellow button) and a shelf of slips on the right. It is a printed, tactile, daytime-lit world: card stock, ink, sticker dots, rubber stamps. It is flat and outlined, not glossy, and it never looks like a table of filenames over a settings sidebar.

Density is moderate and friendly. Only two things are asked of the person: Start and Stop. Everything else is a chip, a segmented choice, or a folder. Tool names and flags never appear.

The three light themes (grey default, mint, lilac) change only the surface tints. Ink and every accent are fixed. Warm cream grounds are refused.

**Key Characteristics:**
- Ink-outlined paper shapes on a tinted ground; 2px ink borders are the default edge.
- One loud sign face (Bowlby One) for brand, Make and stamps; one plain humanist sans (Atkinson Hyperlegible) for everything else.
- Three accents, each with exactly one job: yellow, magenta, stamp green (plus alert red for problems).
- Times and counts use tabular figures.
- Light themes only.

## Colors

A cool, quiet ground with near-black ink, so the few saturated accents read as shop-window stickers.

### Primary
- **Sticker Yellow** (#FFD60A; hover #FFE04D): the Make button, the progress fill, and the Make-styled "Try again" on the setup screen. Nowhere else.

### Secondary
- **Eye Magenta** (#E6007E): only the 3px ring and the notches on the one slip under the eye. Never a button, text or fill.
- **Stamp Green** (#00805A): the DONE stamp only. It is a text-weight green so it holds 3px-border legibility on slip stock.

### Neutral
- **Ink** (#1E1B24): text, every border, focus rings, pressed chips and segments. Same in all themes.
- **Ink 2** (#4A4655): secondary text, dashed dividers, the waiting stamp.
- **Hint** (#6A6676): placeholders.
- **Paper** (#FFFFFF): fields, chips at rest, stickers, badges, hover fills.
- **Alert Red** (#C23B22): invalid time borders, error notes, the bad stamp.
- **Themed surfaces** (set by `data-theme` on the root; grey is the default): `--ground` the window behind slips, `--slip` card stock and the counter, `--line` hairlines and selection, `--tb` the sticky shelf header.
  - Grey: ground #E4E8ED, slip #F8FAFC, line #CBD1D9, tb #DCE1E8
  - Mint: ground #CDEBE4, slip #F7FCFB, line #9BC7BD, tb #B8DDD5
  - Lilac: ground #E4DEF6, slip #FBF9FF, line #BDB3DB, tb #D4CCEB

### Named Rules
**The Reserved Yellow Rule.** Yellow means "press this" or "this is working". It appears on Make, the progress fill, and the setup Try again button, and on nothing else.

**The One Under The Eye Rule.** Magenta appears once per screen at most: the ring on the single slip being looked at.

**The Fixed Ink Rule.** Themes retint ground, slip, line and header only. Ink and accents never change per theme.

## Typography

**Display Font:** Bowlby One (with Arial Black, sans-serif), bundled woff2, OFL
**Body Font:** Atkinson Hyperlegible (with Segoe UI, sans-serif), 400 and 700 bundled woff2, OFL

**Character:** A fat shop-sign face for the few shouted words, a deliberately legible humanist sans for everything a person must read or type.

### Hierarchy
- **Logo** (Bowlby One 400, 26px, 1; 34px on setup, 22px under 640): the wordmark.
- **Sign** (Bowlby One 400, 18px, 1, +0.02em): the Make button label.
- **Stamp** (Bowlby One 400, 15px, 1, +0.06em, uppercase; 13px for the waiting stamp): DONE, failure and waiting marks.
- **Headline** (Atkinson 700, 22px, 1.25): setup screen heading.
- **Title** (Atkinson 700, 15px, 1.25): slip titles, shelf title; settings heading is 16px.
- **Body** (Atkinson 400, 15px, 1.4): base; notes and sources 13px.
- **Label** (Atkinson 700, 12px): field group labels; buttons and chips 14px bold.
- **Time** (Atkinson 700, 19px, +0.02em, tabular figures): Start and Stop boxes, 92px wide, centred. Counts, lengths and percentages are tabular too.

### Named Rules
**The Sign Is Rare Rule.** Bowlby One is for the wordmark, Make and stamps. Body copy, titles and controls stay in Atkinson.

## Layout

Two columns: a 292px counter (scrolls on its own, Make sticky at its bottom) and a flexible shelf. Shelf slips stack in a single column with 14px gaps; the shelf header is sticky over the scrolling list. Padding rhythm is 6 / 8 / 14 / 18 / 28px; the shelf has 20px side padding.

Breakpoints: under 900px the counter narrows to 240px, status moves under the times, and the inline preview becomes one column. Under 640px the layout becomes one scrolling column: compact counter on top (labels beside controls, drop area on one row), shelf below, Make fixed to the bottom edge at 56px, and each slip's stub becomes a 64px horizontal band with the perforation running along its bottom.

Every control is at least 32px tall (36px for buttons and fields, 50px for Make). Times are in fixed-width boxes so columns align down the shelf.

## Elevation & Depth

Flat by default. Depth comes from ink outlines, paper-on-tint layering and the perforated notches, not from shadows. There are no hard-offset shadows.

### Shadow Vocabulary
- **Make rest** (`box-shadow: 0 8px 14px -8px rgba(30,27,36,.5)`): soft lift under the one primary action; shrinks to `0 3px 8px -6px` on press with a 2px drop. Fixed bottom bar uses `0 -6px 14px -8px rgba(30,27,36,.4)`.
- **Settings popover** (`0 14px 30px -12px rgba(30,27,36,.45), 0 2px 6px rgba(30,27,36,.12)`): the only floating surface.
- **Eye ring** (`0 0 0 3px #E6007E`): a ring, not a shadow; see Colors.

### Named Rules
**The No Offset Rule.** No hard offset shadows. Soft, tall-and-narrow shadows only, and only for Make and the popover.

## Shapes

Rounded-rectangle paper with a firm ink outline: 10px slips and popovers, 8px fields and buttons, 12px Make, 6px stamps and time boxes, 4px thumbnails, full pills (99px) for chips, stickers, badges, tracks and switches. Borders are 2px ink (1.5px on slips and the setup card, 3px on Make and stamps).

The signature silhouette is the slip: a card with a 92px stub on the left, a dashed perforation (ink-2, 3px dashes), and two half-circle notches cut at the top and bottom of the perforation in the ground colour. Dashed borders mean "not yet" (drop target at rest, waiting stamp, empty time box, disabled Make).

## Components

### Buttons
- **Make:** yellow, Bowlby One 18px, 3px ink border, 12px radius, min 50px. Hover lightens to #FFE04D; active drops 2px. Disabled becomes slip-coloured with a dashed border and ink-2 text.
- **Standard (`btn`):** slip fill, 2px ink border, 8px radius, bold 14px, min 36px; hover fills paper.
- **Quiet:** transparent, underlined, min 32px; hover thickens the underline.
- **Icon-only:** transparent until hover or expanded, then paper with ink border.

### Chips and segments
Pill chips (format choice, two columns) and a bordered segmented control (quality). At rest paper; pressed is solid ink with slip-coloured text. Hover tints to ground.

### Inputs
Paper fill, 2px ink border, 8px radius, hint-coloured placeholder. Time boxes: 19px bold tabular, dashed border when a value is still needed, alert-red border when invalid. Focus everywhere: 3px ink outline, 2px offset (inset on segments).

### Slip
Perforated card described in Shapes: thumbnail and length on the stub, title, source, Start and Stop and status on the main side. A finished slip opens its preview inline (a dashed top rule, a 16:10 player, format tabs, facts, actions), never a modal. The slip under the eye gets the magenta ring.

### Sticker
Paper pill with ink border and tabular figure, 30px: counts. Badges on previews are the same paper pill. Stickers and badges are paper, never coloured.

### Stamp
Bowlby One, 3px currentColor border, 6px radius, tilted -6deg. DONE is stamp green and slams in (scale 1.9 to 1 over .5s, ease-out `cubic-bezier(.16,1,.3,1)`); bad is alert red; waiting is ink-2, dashed, -2deg. A stamp that is a button gets a paper hover.

### Progress
One bar plus a stage label above it, never two bars. Measured stages fill yellow from the left (14px pill track, ink border). Unmeasured stages pulse the fill; unknown clip length sweeps a block across the track. Under `prefers-reduced-motion` the slam, pulse and sweep stop and unmeasured stages show a static striped yellow/ink fill.

### Settings popover
Top-layer popover under the gear, 340px max: three theme swatches (40px split ground/slip dots, selected = double ring), choices and switches (46x26 pill switch, ink when on).

## Do's and Don'ts

### Do:
- **Do** keep yellow (#FFD60A) to Make, the progress fill and the setup Try again.
- **Do** put magenta (#E6007E) on one slip only, as a ring.
- **Do** make stickers, badges and counts paper with an ink border.
- **Do** use `--ground`, `--slip`, `--line`, `--tb` for any surface tint so all three themes work.
- **Do** keep every control at least 32px tall and give focus a 3px ink outline.
- **Do** use tabular figures for every time, count and percentage.
- **Do** keep stamps in Bowlby One with the slam-in, and remove the animation under reduced motion.
- **Do** use the breakpoints 900px and 640px.

### Don't:
- **Don't** use hard-offset shadows or coloured shadows.
- **Don't** tint a theme warm or cream, or add a dark theme.
- **Don't** introduce a second progress bar or a percentage-only readout without the stage label.
- **Don't** open a modal for preview; it lives inside the slip.
- **Don't** show tool names, flags or filenames-in-a-table as the primary layout.
- **Don't** use Bowlby One for body copy or controls.
