package runner

// Seconds of work per stage, so the bar moves by how long each stage really
// takes. Measured in Task 4b (2026-10-19): Windows 11, Ryzen 9 7900X, one home
// connection, Big Buck Bunny, 10 s clip, median of 3 runs. Ballpark only:
// re-measure if the download or encode settings change.

// findCost is fixed seconds for finding the video.
const findCost = 2.0

// perSecond is seconds per second of clip, keyed "<quality>/<stage>".
// "download" is the smaller gif/webp-only source; "download-full" is the
// best-available source (FullSource on, or any mp4/mov in the run).
var perSecond = map[string]float64{
	"low/download":    0.27,
	"medium/download": 0.86,
	"high/download":   0.93,

	"low/download-full":    1.13,
	"medium/download-full": 1.14,
	"high/download-full":   1.14,

	"low/gif":    0.066,
	"medium/gif": 0.163,
	"high/gif":   0.90,

	"low/webp":    0.025,
	"medium/webp": 0.072,
	"high/webp":   0.275,

	"low/mp4":    0.57,
	"medium/mp4": 0.59,
	"high/mp4":   0.64,

	"low/mov":    0.57,
	"medium/mov": 0.59,
	"high/mov":   0.64,
}
