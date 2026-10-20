// Pure slip-list logic for the main window; app.js does the DOM. A slip is an
// appcore.Slip plus the page's own fields:
//   pending   placeholder from Add, waiting for slips:replace
//   source    playlist title or "Pasted links"
//   startText, endText  what the user typed
//   start     seconds from Validate, null until a valid Start is set
//   end       seconds, 0 = to the end
//   timeMsg   Validate's message, "" = fine
//   checked   "startText|endText" as last validated; start/end belong to it
//   status    null (ready) or {kind, stage, pct, stagePct, message, files}
// Every function returns a new array; the input is never changed.

const blank = { source: "", startText: "", endText: "", start: null, end: 0, timeMsg: "", checked: "|", status: null };

// addPlaceholders appends Add's placeholders, ignoring links already on the shelf.
export function addPlaceholders(slips, newSlips) {
  const seen = new Set(slips.map((s) => s.url));
  const out = [...slips];
  for (const p of newSlips) {
    if (seen.has(p.url)) continue;
    seen.add(p.url);
    out.push({ ...blank, ...p, pending: true });
  }
  return out;
}

// replaceSlip swaps the placeholder id for newSlips in place. Videos already on
// the shelf are dropped (entries without a link, e.g. private ones, are kept).
// An unknown id returns slips itself, so the caller can tell.
export function replaceSlip(slips, id, newSlips) {
  const i = slips.findIndex((s) => s.id === id);
  if (i < 0) return slips;
  const seen = new Set(slips.filter((s) => s.id !== id).map((s) => s.url));
  const fresh = [];
  for (const n of newSlips || []) {
    if (n.url && seen.has(n.url)) continue;
    if (n.url) seen.add(n.url);
    fresh.push({ ...blank, ...n });
  }
  return [...slips.slice(0, i), ...fresh, ...slips.slice(i + 1)];
}

// canRetry: a lookup that failed and still has its link, so it can be looked
// up again (a private entry inside a playlist has no link of its own).
export const canRetry = (s) => !!(s.failed && s.url);

// retrySlip puts a fresh placeholder (from Add) where the failed slip was, so
// the usual slips:replace fills it in. Anything that can't be retried returns
// slips itself.
export function retrySlip(slips, id, placeholder) {
  const i = slips.findIndex((s) => s.id === id);
  if (i < 0 || !canRetry(slips[i])) return slips;
  return [...slips.slice(0, i), { ...blank, ...placeholder, pending: true }, ...slips.slice(i + 1)];
}

// canRun: a real video that isn't in the current run. A finished slip can be
// made again (another quality or format makes a new file; the same one is
// answered "already" before any download).
const canRun = (s) => !s.pending && !s.unavailable && !s.failed && !["queued", "making"].includes(s.status?.kind);
const typed = (s) => s.startText + "|" + s.endText;
const isReady = (s) => canRun(s) && s.start !== null && !s.timeMsg && s.checked === typed(s);

// needsCheck: slips whose typed times haven't been validated yet; Make
// validates these first so it never sends stale start/end.
export function needsCheck(slips) {
  return slips.filter((s) => canRun(s) && s.startText.trim() && s.checked !== typed(s)).map((s) => s.id);
}

// makeCount: what the Make label counts, ready slips plus ones Make will check first.
export function makeCount(slips) {
  return readyJobs(slips).length + needsCheck(slips).length;
}

// readyJobs is what Make sends: runner.Job for each slip that can be made with
// validated times.
export function readyJobs(slips) {
  return slips.filter(isReady).map((s) => ({
    id: s.id, url: s.url, title: s.title, start: s.start, end: s.end, durationSec: s.durationSec,
  }));
}

const update = (slips, id, fn) => {
  const i = slips.findIndex((s) => s.id === id);
  if (i < 0) return slips;
  const out = [...slips];
  out[i] = fn(slips[i]);
  return out;
};

// applyStatus folds one slip:status event into the list. Files always come
// from the event: a new run's result replaces the previous run's.
export function applyStatus(slips, e) {
  return update(slips, e.jobId, (s) => {
    if (e.kind === "ready") return { ...s, status: null };
    const files = e.files || [];
    if (e.kind === "making") {
      const prev = s.status?.kind === "making" ? s.status.pct : -1;
      const pct = e.pct < 0 ? -1 : Math.max(prev, e.pct);
      return { ...s, status: { kind: "making", stage: e.stage, pct, stagePct: e.stagePct, message: "", files } };
    }
    return { ...s, status: { kind: e.kind, stage: "", pct: e.pct, stagePct: -1, message: e.message || "", files } };
  });
}

// startRun shows "Next up" on the slips sent to Make.
export function startRun(slips, ids) {
  const set = new Set(ids);
  return slips.map((s) => (set.has(s.id) ? { ...s, status: { kind: "queued", files: [] } } : s));
}

// endRun puts slips the run never finished back to ready (Stop, or run:finished).
export function endRun(slips) {
  return slips.map((s) => (s.status && ["queued", "making"].includes(s.status.kind) ? { ...s, status: null } : s));
}

// eyeId is the one slip under the magenta ring: the previewed one, else the
// one being made, else null.
export function eyeId(slips, previewId) {
  if (previewId && slips.some((s) => s.id === previewId && s.status?.files?.length)) return previewId;
  return slips.find((s) => s.status?.kind === "making")?.id ?? null;
}

export function makeLabel(n) {
  if (n === 0) return "Make clips";
  return n === 1 ? "Make 1 clip" : `Make ${n} clips`;
}

export function formatDuration(sec) {
  if (!(sec > 0)) return "—";
  const t = Math.floor(sec);
  const h = Math.floor(t / 3600), m = Math.floor((t % 3600) / 60), s = t % 60;
  const ss = String(s).padStart(2, "0");
  return h ? `${h}:${String(m).padStart(2, "0")}:${ss}` : `${m}:${ss}`;
}

const STAGES = {
  find: "Finding the video", download: "Getting the clip",
  gif: "Making GIF", webp: "Making WebP", mp4: "Making MP4", mov: "Making MOV",
};

// stageLabel: a percentage only for a measured stage (stagePct >= 0), never 100%.
export function stageLabel(e) {
  const name = STAGES[e.stage] || "Working on it";
  if (!(e.stagePct >= 0)) return name + "…";
  return `${name} ${Math.min(99, Math.round(e.stagePct * 100))}%`;
}

// barValue is the single number behind a bar's fill, aria-valuenow and label:
// null when indeterminate, else at most 0.99 (only "done" is full).
export function barValue(pct) {
  return pct >= 0 ? Math.min(pct, 0.99) : null;
}

// sourceTitle: "Family videos" or "Family videos, plus pasted links".
export function sourceTitle(slips) {
  const names = [...new Set(slips.map((s) => s.source).filter(Boolean))];
  return names.map((n, i) => (i && n === "Pasted links" ? "pasted links" : n)).join(", plus ");
}

// setupProgress turns per-tool download events into one bar.
// ponytail: each of the 3 tools counts the same; weight by size if the bar feels uneven.
const TOOL_COUNT = 3;
export function setupProgress(acc = { tools: [], pct: 0 }, e) {
  const tools = acc.tools.includes(e.tool) ? acc.tools : [...acc.tools, e.tool];
  const frac = e.total > 0 ? Math.min(e.done / e.total, 1) : 0;
  const pct = Math.min(0.99, Math.max(acc.pct, (tools.length - 1 + frac) / TOOL_COUNT));
  return { tools, pct, known: e.total > 0 };
}

export function formatSize(bytes) {
  if (bytes >= 1024 * 1024) return (bytes / 1024 / 1024).toFixed(1) + " MB";
  return Math.max(1, Math.round(bytes / 1024)) + " KB";
}

// clipLength: Stop minus Start, or the video's length minus Start; 0 = unknown.
export function clipLength(s) {
  if (s.end > 0) return s.end - (s.start || 0);
  return s.durationSec > (s.start || 0) ? s.durationSec - (s.start || 0) : 0;
}
