# Fonts

Bundled so the app needs no network at runtime. Latin subset only.

| File | Font | Source | Licence |
|---|---|---|---|
| `bowlby-one-latin-400-normal.woff2` | Bowlby One 400 (logo, stamps) | npm `@fontsource/bowlby-one@5.3.0` (Google Fonts v25) | SIL Open Font License 1.1, copyright 2011 Vernon Adams; full text in `OFL-bowlby-one.txt` |
| `atkinson-hyperlegible-latin-400-normal.woff2`, `atkinson-hyperlegible-latin-700-normal.woff2` | Atkinson Hyperlegible 400 and 700 (everything else) | npm `@fontsource/atkinson-hyperlegible@5.3.0` (Google Fonts v12) | SIL Open Font License 1.1, copyright 2020 Braille Institute of America; full text in `OFL-atkinson-hyperlegible.txt` |

Atkinson's digits are proportional by default but have a working `tnum`
feature: with `font-variant-numeric: tabular-nums`, `1111`, `8888` and `0000`
all measure 48.05 px at 700 19px (without it: 34.06, 47.50, 49.56). So times and
sizes use Atkinson with `tabular-nums`, and no third font is bundled.
