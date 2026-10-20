// Main window behaviour: wires the page to the Go side (window.go.main.App,
// events in internal/appcore). Logic that can be pure lives in state.js.
// Remote text (titles, links, messages) only ever goes in via textContent or
// attributes, never innerHTML.
import * as S from "./state.js";

const App = window.go.main.App;
const on = (name, cb) => window.runtime.EventsOn(name, cb);
const $ = (id) => document.getElementById(id);
const appEl = document.querySelector(".app");
const sheet = $("sheet");
const makeBtn = $("make");
const linkField = $("link");

const FMT = { gif: "GIF", webp: "WebP", mp4: "MP4", mov: "MOV" };
const NEED_START = "Add a start time to include this one.";
const GONE = "Couldn't get this video. It may be private or removed.";
const ADMIN = "The helper files couldn't be set up for your account. Ask an administrator to run Repair for vid2gif in Windows' installed apps, or reinstall it for just you.";

let slips = [];
let settings = null;
let binDir = "";
let running = false;
let previewId = null;
const previewTab = new Map(); // slip id -> format shown in its preview
const buffered = new Map(); // slips:replace that arrived before Add returned
const dropped = new Set(); // placeholders removed before Add returned: their replace is ignored
const rows = new Map(); // slip id -> <li>

// ---------- tiny DOM helpers ----------
function el(tag, props = {}, ...kids) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (v == null || v === false) continue;
    if (k === "class") n.className = v;
    else if (k === "text") n.textContent = v;
    else if (k.startsWith("on")) n.addEventListener(k.slice(2), v);
    else n.setAttribute(k, v === true ? "" : v);
  }
  n.append(...kids.filter((x) => x != null && x !== false));
  return n;
}
function icon(id, cls) {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  if (cls) svg.setAttribute("class", cls);
  svg.setAttribute("aria-hidden", "true");
  const use = document.createElementNS(ns, "use");
  use.setAttribute("href", "#" + id);
  svg.append(use);
  return svg;
}
const isHttps = (u) => { try { return new URL(u).protocol === "https:"; } catch { return false; } };
const shortUrl = (u) => u.replace(/^https?:\/\/(www\.)?/, "");
const seconds = (n) => {
  const r = Math.round(n * 10) / 10;
  return r < 60 ? `${r} second${r === 1 ? "" : "s"}` : S.formatDuration(n);
};

let noticeTimer;
function notice(text) {
  const n = $("notice");
  n.textContent = text;
  n.hidden = !text;
  clearTimeout(noticeTimer);
  if (text) noticeTimer = setTimeout(() => (n.hidden = true), 8000);
}

// one value drives the fill, aria-valuenow and (via the caller) the label
function setBar(node, pct, label, pulse) {
  const v = S.barValue(pct);
  node.classList.toggle("indeterminate", v === null);
  node.classList.toggle("pulse", v !== null && pulse);
  if (v === null) {
    node.removeAttribute("aria-valuenow");
    node.style.removeProperty("--pct");
  } else {
    node.setAttribute("aria-valuenow", String(Math.round(v * 100)));
    node.style.setProperty("--pct", String(v));
  }
  node.setAttribute("aria-valuetext", label);
  node.querySelector(".stage").textContent = label;
}
const progressEl = (label) => el("div", { class: "progress", role: "progressbar", "aria-label": label, "aria-valuemin": "0", "aria-valuemax": "100" },
  el("span", { class: "stage" }), el("div", { class: "track" }, el("div", { class: "fill" })));

// ---------- rendering ----------
let frame = 0;
function render() {
  if (!frame) frame = requestAnimationFrame(() => { frame = 0; paintAll(); });
}

const isBusy = (s) => ["queued", "making"].includes(s?.status?.kind); // already sent to Make
// markable: a Later slip that can't clearly fail. Typed-but-unchecked times
// count as fine, so Tab and a quick click reach Mark ready before the blur check
// returns; markReady checks them first and leaves a bad one in Later.
const markable = (s) => s.lane === "later" && !s.pending && !s.unavailable && !s.failed && !!s.startText.trim()
  && (s.checked !== S.typed(s) || !s.timeMsg);
let focusLane = null; // after a slip changes lane, focus its lane button in the new place
let focusStart = null; // after Make again, focus the new Later slip's Start box
let showAllMade = false; // Made before shows 5 lines until Show more
const madeRows = new Map(); // made slip id -> <li> in the log
const expanded = new Set(); // made lines opened without a preview (no files to play)

function paintAll() {
  const shown = slips.filter((s) => s.lane !== "made"); // Made before is drawn apart
  const madeShown = S.madeVisible(slips, showAllMade);
  // a preview belongs to a slip on the shelf or a visible line of the log
  const eye = S.eyeId([...shown, ...madeShown], previewId);
  if (previewId && eye !== previewId) previewId = null;
  const header = S.sourceTitle(shown);
  const ids = new Set(shown.map((s) => s.id));
  for (const [id, li] of rows) if (!ids.has(id)) { li.remove(); rows.delete(id); }
  for (const lane of ["ready", "later"]) {
    const list = $("list-" + lane);
    const inLane = S.inLane(slips, lane);
    let prev = null;
    for (const s of inLane) {
      let li = rows.get(s.id);
      if (!li) { rows.set(s.id, (li = newRow())); li.dataset.id = s.id; }
      paint(li, s, eye, header);
      const want = prev ? prev.nextSibling : list.firstChild;
      if (want !== li) list.insertBefore(li, want);
      prev = li;
    }
    $("n-" + lane).textContent = String(inLane.length);
    $("none-" + lane).hidden = inLane.length > 0;
    $("sec-" + lane).hidden = slips.length === 0;
  }
  $("mark-all").disabled = !slips.some(markable);
  const made = S.inLane(slips, "made").length;
  $("n-made").textContent = String(made);
  $("sec-made").hidden = made === 0;
  paintMade(madeShown, made);
  $("clear-cache").disabled = running;
  $("cache-yes").disabled = running;
  $("cache-wait").hidden = !running;
  if (focusLane) { rows.get(focusLane)?.querySelector(".push button:not([hidden]):not(:disabled)")?.focus(); focusLane = null; }
  if (focusStart) { rows.get(focusStart)?.querySelector("input.start")?.focus(); focusStart = null; }
  $("empty").hidden = slips.length > 0;
  $("source").textContent = header || "Your shelf";
  $("count").textContent = String(shown.length);
  $("count-word").textContent = shown.length === 1 ? "video" : "videos";
  if (running) {
    makeBtn.textContent = "Stop";
    makeBtn.disabled = false;
  } else {
    const n = S.makeCount(slips);
    makeBtn.textContent = S.makeLabel(n);
    makeBtn.disabled = n === 0;
  }
}

function newRow() {
  const li = el("li", { class: "slip" },
    el("div", { class: "stub" }, el("div", { class: "thumb" }), el("div", { class: "len" })),
    el("div", { class: "main" },
      el("div", {}, el("p", { class: "title" }), el("p", { class: "src" })),
      el("div", { class: "row2" })));
  return li;
}

const titleOf = (s) => (s.pending ? "Looking this one up…" : s.title || (s.unavailable ? "Unavailable video" : "Link you pasted"));
const modeOf = (s) => (s.pending ? "pending" : s.failed || s.unavailable ? "gone" : "slip");

function paint(li, s, eye, header) {
  const mode = modeOf(s);
  const title = titleOf(s);
  li.querySelector(".title").textContent = title;
  // a playlist entry names its playlist only when the header doesn't already
  const src = s.source && s.source !== "Pasted links" ? (s.source === header ? "" : s.source) : shortUrl(s.url || "");
  li.querySelector(".src").textContent = src;
  li.querySelector(".src").hidden = !src;
  li.querySelector(".len").textContent = S.formatDuration(s.durationSec);

  const thumbKey = mode + "|" + s.thumbnailUrl;
  if (li._thumb !== thumbKey) {
    li._thumb = thumbKey;
    const t = li.querySelector(".thumb");
    t.replaceChildren();
    if (mode === "gone") t.append(icon("scene-none"));
    else if (isHttps(s.thumbnailUrl)) {
      const img = el("img", { src: s.thumbnailUrl, alt: "", loading: "lazy", referrerpolicy: "no-referrer" });
      img.addEventListener("error", () => img.remove());
      t.append(img);
    }
  }

  const row2 = li.querySelector(".row2");
  if (li._mode !== mode) {
    li._mode = mode;
    li._stateKey = null;
    row2.replaceChildren(...buildRow2(s, mode));
  }
  if (mode === "gone") {
    const msg = s.message || GONE;
    row2.querySelector(".lead .note.alert").textContent = msg;
    // the privacy hint only makes sense for private or removed videos; a link
    // with nothing to look up again (a private entry in a playlist) has no button
    row2.querySelector(".lead .note.hint").hidden = !(S.canRetry(s) && msg === GONE);
    row2.querySelector("button.retry").hidden = !S.canRetry(s);
  }
  if (mode === "slip") { paintTimes(li, s, title); paintOptions(li, s); paintLane(li, s, title); }
  li.classList.toggle("later", s.lane === "later");

  li.classList.toggle("eye", s.id === eye);
  const open = s.id === previewId;
  li.classList.toggle("open", open);
  paintPreview(li, s, open);
}

function buildRow2(s, mode) {
  if (mode === "pending") return [el("div", { class: "state lead" }, el("span", { class: "stamp wait", text: "Looking up" }))];
  if (mode === "gone") {
    return [
      el("div", { class: "state lead" },
        el("p", { class: "note alert" }),
        el("p", { class: "note hint", text: "If you've changed its privacy settings, press Try again." })),
      el("div", { class: "state" },
        el("span", { class: "stamp bad", text: "Not found" }),
        el("div", { class: "actions" }, retryBtn(s.id), removeBtn(s.id))),
    ];
  }
  const input = (which, placeholder) => el("input", {
    class: which, inputmode: "decimal", placeholder, autocomplete: "off", spellcheck: "false",
    oninput: (e) => { slips = slips.map((x) => (x.id === s.id ? { ...x, [which + "Text"]: e.target.value } : x)); persist(s.id); render(); },
    onblur: () => validate(s.id, which),
    onkeydown: (e) => { if (e.key === "Enter") validate(s.id, which); },
  });
  // "to" takes a stop time, "for" takes a length; paintTimes keeps the label,
  // placeholder and value in step with the slip.
  const join = el("select", {
    class: "join", title: "Choose where the clip stops, or how long it runs",
    onchange: (e) => { slips = S.setEndKind(slips, s.id, e.target.value); persist(s.id); render(); validate(s.id, "end"); },
  }, el("option", { value: "stop", text: "to" }), el("option", { value: "length", text: "for" }));
  return [
    el("label", { class: "time" }, "Start", input("start", "0:00")),
    join,
    el("label", { class: "time" }, el("span", { class: "lab", text: "Stop" }), input("end", "end")),
    el("div", { class: "state" }),
    el("div", { class: "push" },
      el("button", { class: "btn quiet to-later", type: "button", text: "Move to Later", onclick: () => moveToLater(s.id) }),
      el("button", { class: "btn to-ready", type: "button", text: "Mark ready", onclick: async () => { const [id] = await markReady([s.id]); if (id) { focusLane = id; render(); } } }),
      el("button", { class: "btn quiet rm", type: "button", text: "Remove", onclick: () => remove(s.id) })),
    optionsBox(),
  ];
}

// paintLane: Ready slips offer Move to Later (not while queued or making, when
// the slip is already on its way to Make); Later slips offer Mark ready, which
// waits until the slip would pass Make's own check.
function paintLane(li, s, title) {
  const [toLater, toReady] = [li.querySelector(".to-later"), li.querySelector(".to-ready")];
  toLater.hidden = s.lane !== "ready" || isBusy(s);
  toLater.setAttribute("aria-label", "Move to Later, " + title);
  toReady.hidden = s.lane !== "later";
  toReady.disabled = !markable(s);
  toReady.setAttribute("aria-label", "Mark ready, " + title);
  const rm = li.querySelector(".rm");
  rm.hidden = isBusy(s);
  rm.setAttribute("aria-label", "Remove " + title);
}

// markReady moves the named Later slips that pass Make's check to Ready and
// saves them. Typed times not yet checked are checked first, as Make does; a
// slip that fails stays in Later with its time message showing. Returns the
// ids that moved.
async function markReady(ids) {
  const field = (id) => slips.find((x) => x.id === id)?.badField || "start";
  await Promise.all(ids.filter((id) => {
    const s = slips.find((x) => x.id === id);
    return s && markable(s) && s.checked !== S.typed(s);
  }).map((id) => validate(id, field(id))));
  const before = new Set(S.inLane(slips, "ready").map((s) => s.id));
  slips = S.markReady(slips, ids);
  const moved = S.inLane(slips, "ready").map((s) => s.id).filter((id) => !before.has(id));
  persist(...moved);
  render();
  return moved;
}

// moveToLater parks a Ready slip and saves it; refused once it is on its way to Make.
function moveToLater(id) {
  if (isBusy(slips.find((s) => s.id === id))) return;
  slips = S.moveToLater(slips, id);
  persist(id);
  focusLane = id;
  render();
}

// optionsBox is a slip's own formats and quality, hidden under a one-line
// summary; paintOptions fills it in. Clicks are handled once on the shelf.
const QUALITY = { low: "Small file", medium: "Balanced", high: "Sharp" };
function optionsBox() {
  return el("details", { class: "opts" },
    el("summary", {}),
    el("div", { class: "group", role: "group", "aria-label": "Make this one as" },
      el("div", { class: "chips" }, ...Object.entries(FMT).map(([f, name]) =>
        el("button", { class: "chip", type: "button", "data-format": f, text: name }))),
      el("div", { class: "seg" }, ...Object.entries(QUALITY).map(([q, name]) =>
        el("button", { type: "button", "data-quality": q, text: name })))));
}

function paintOptions(li, s) {
  const box = li.querySelector(".opts");
  const quality = s.quality || settings.quality;
  const formats = s.formats?.length ? s.formats : settings.formats;
  box.querySelector("summary").textContent = `${formats.map((f) => FMT[f]).join(" + ")} · ${QUALITY[quality]}`;
  for (const b of box.querySelectorAll(".chip")) b.setAttribute("aria-pressed", String(formats.includes(b.dataset.format)));
  for (const b of box.querySelectorAll(".seg button")) b.setAttribute("aria-pressed", String(b.dataset.quality === quality));
  box.inert = isBusy(s);
}

sheet.addEventListener("click", (e) => {
  const b = e.target.closest(".opts button");
  const id = b?.closest("li")?.dataset.id;
  const s = slips.find((x) => x.id === id);
  if (!s) return;
  if (b.dataset.format) {
    const formats = S.toggleFormat(s.formats, b.dataset.format);
    if (formats === s.formats) return notice("At least one format stays on.");
    slips = S.setOptions(slips, id, { formats });
  } else {
    slips = S.setOptions(slips, id, { quality: b.dataset.quality });
  }
  persist(id);
  render();
});

const removeBtn = (id) => el("button", { class: "btn quiet", type: "button", text: "Remove", onclick: () => remove(id) });
const retryBtn = (id) => el("button", { class: "btn retry", type: "button", text: "Try again", onclick: () => retry(id) });

function paintTimes(li, s, title) {
  const [start, end] = [li.querySelector("input.start"), li.querySelector("input.end")];
  // once sent to Make the times are fixed: a late edit would save a stale "ready" clip
  start.readOnly = end.readOnly = isBusy(s);
  li.querySelector("select.join").disabled = isBusy(s);
  if (document.activeElement !== start && start.value !== s.startText) start.value = s.startText;
  if (document.activeElement !== end && end.value !== s.endText) end.value = s.endText;
  start.setAttribute("aria-label", "Start, " + title);
  const len = s.endKind === "length";
  const join = li.querySelector("select.join");
  join.value = len ? "length" : "stop";
  join.setAttribute("aria-label", "Stop time or length, " + title);
  li.querySelector(".time .lab").textContent = len ? "Length" : "Stop";
  end.placeholder = len ? "to end" : "end";
  end.setAttribute("aria-label", (len ? "Length, " : "Stop, ") + title);

  const needStart = !s.startText.trim();
  const msg = needStart ? NEED_START : s.timeMsg;
  const bad = msg && !needStart ? (/^(Stop|Length)/.test(s.timeMsg) ? "end" : s.badField || "start") : "";
  start.parentElement.classList.toggle("need", needStart);
  for (const [inp, which] of [[start, "start"], [end, "end"]]) {
    if (bad === which) inp.setAttribute("aria-invalid", "true");
    else inp.removeAttribute("aria-invalid");
    if (msg) inp.setAttribute("aria-describedby", "msg-" + s.id);
    else inp.removeAttribute("aria-describedby");
  }

  const st = s.status;
  const key = JSON.stringify([msg, needStart, st?.kind, st?.kind === "making" ? "" : st?.message,
    (st?.files || []).map((f) => f.format + f.url), settings?.debugMode]);
  const box = li.querySelector(".state");
  if (li._stateKey !== key) {
    li._stateKey = key;
    box.replaceChildren(...buildState(s, msg, needStart));
  }
  if (st?.kind === "making") setBar(box.querySelector(".progress"), st.pct, S.stageLabel(st), !(st.stagePct >= 0));
  const stamp = box.querySelector("button.stamp");
  if (stamp) {
    const open = s.id === previewId;
    const what = st.kind === "already" ? "Already in your folder." : "Done.";
    stamp.setAttribute("aria-expanded", String(open));
    stamp.setAttribute("aria-label", `${what} ${open ? "Close" : "Open"} the preview`);
    if (open) stamp.setAttribute("aria-controls", "preview-" + s.id);
    else stamp.removeAttribute("aria-controls");
  }
  const watch = box.querySelector("button.watch");
  if (watch) {
    const open = s.id === previewId;
    watch.hidden = open;
    watch.setAttribute("aria-expanded", String(open));
    if (open) watch.setAttribute("aria-controls", "preview-" + s.id);
    else watch.removeAttribute("aria-controls");
  }
}

// under the formats line: Watch opens the preview; with nothing playable
// (a MOV-only result) Open folder instead; no files, nothing
function finishedAction(s, files) {
  if (files.some(playable)) {
    return el("button", {
      class: "btn quiet watch", type: "button", text: "Watch", "aria-label": "Watch " + titleOf(s),
      onclick: () => { previewId = s.id; paintAll(); rows.get(s.id)?.querySelector("button.stamp")?.focus(); },
    });
  }
  if (files.length) return el("button", { class: "btn quiet", type: "button", text: "Open folder", onclick: () => App.OpenFolder(files[0].path) });
  return null;
}

function buildState(s, msg, needStart) {
  const out = [];
  if (msg) out.push(el("p", { class: needStart ? "note" : "note alert", id: "msg-" + s.id, text: msg }));
  const st = s.status;
  if (!st) return out;
  switch (st.kind) {
    case "queued":
      out.push(el("span", { class: "stamp wait", text: "Next up" }));
      break;
    case "making":
      out.push(progressEl("Making " + titleOf(s)));
      break;
    case "done": case "partial": case "already":
      out.push(el("button", {
        class: "stamp done", type: "button", text: st.kind === "already" ? "Returned" : "Done",
        onclick: () => { previewId = previewId === s.id ? null : s.id; render(); },
      }));
      if (st.kind !== "already") out.push(el("p", { class: "files", text: st.files.map((f) => FMT[f.format] || f.format.toUpperCase()).join(" + ") }));
      if (st.kind === "already") out.push(el("p", { class: "note", text: "Already in your folder" }));
      { const act = finishedAction(s, st.files); if (act) out.push(act); }
      if (st.kind === "partial" && st.message) out.push(el("p", { class: "note alert", text: st.message }));
      break;
    case "failed":
      out.push(el("p", { class: "note alert", text: st.message }));
      out.push(el("span", { class: "stamp bad", text: st.message === GONE ? "Not found" : "Not made" }));
      out.push(el("div", { class: "actions" },
        settings?.debugMode ? el("button", { class: "btn quiet", type: "button", text: "Show log", onclick: () => App.OpenLogs() }) : null,
        removeBtn(s.id)));
      break;
  }
  return out;
}

// ---------- preview inside the slip ----------
const playable = (f) => f && typeof f.url === "string" && f.url.startsWith("/out/") && ["gif", "webp", "mp4"].includes(f.format);

function paintPreview(li, s, open) {
  const files = s.status?.files || [];
  const old = li.querySelector(".inline-preview");
  if (!open || !files.length) {
    if (old) old.remove();
    li._pkey = null;
    return;
  }
  const f = files.find((x) => x.format === previewTab.get(s.id)) || files.find(playable) || files[0];
  const key = f.format + "|" + files.map((x) => x.url).join();
  if (li._pkey === key) return;
  li._pkey = key;
  const region = buildPreview(s, files, f);
  if (old) old.replaceWith(region); else li.append(region);
}

function buildPreview(s, files, f) {
  const title = titleOf(s);
  const len = S.clipLength(s);
  const lenDd = el("dd", { text: len ? seconds(len) : "—" });
  const widthDd = el("dd", { text: "—" });
  let player = null;
  if (playable(f)) {
    let media;
    if (f.format === "mp4") {
      media = el("video", { src: f.url, autoplay: true, loop: true, muted: true, playsinline: true, controls: true, "aria-label": `MP4 clip of ${title}` });
      media.muted = true;
      media.addEventListener("loadedmetadata", () => {
        widthDd.textContent = media.videoWidth + " px";
        if (!len && media.duration > 0) lenDd.textContent = seconds(media.duration);
      });
    } else {
      media = el("img", { src: f.url, alt: `${FMT[f.format]} clip of ${title}` });
      media.addEventListener("load", () => { widthDd.textContent = media.naturalWidth + " px"; });
    }
    player = el("div", { class: "player" }, media, el("span", { class: "badge", text: FMT[f.format] }),
      f.format !== "mp4" && len ? el("span", { class: "loop", text: S.formatDuration(len) + " loop" }) : null);
  }
  const tabs = files.length > 1 ? el("div", { class: "fmt-tabs", role: "group", "aria-label": "Format" },
    ...files.map((x) => el("button", {
      class: "chip", type: "button", "aria-pressed": String(x === f), "data-format": x.format, text: FMT[x.format] || x.format,
      onclick: () => {
        previewTab.set(s.id, x.format);
        paintAll();
        (rows.get(s.id) || madeRows.get(s.id))?.querySelector(`.fmt-tabs [data-format="${x.format}"]`)?.focus();
      },
    }))) : null;
  return el("div", { class: "inline-preview" + (player ? "" : " no-player"), id: "preview-" + s.id, role: "region", "aria-label": "Preview of " + title },
    player,
    el("div", { class: "info" },
      tabs,
      player ? null : el("p", { class: "note", text: `The ${FMT[f.format] || f.format} plays outside the app. Open the folder to watch it.` }),
      el("dl", { class: "facts" },
        el("dt", { text: "Length" }), lenDd,
        el("dt", { text: "Size" }), el("dd", { text: S.formatSize(f.size) }),
        ...(player ? [el("dt", { text: "Width" }), widthDd] : [])),
      el("div", { class: "actions" },
        el("button", { class: "btn", type: "button", onclick: () => App.OpenFolder(f.path) }, icon("i-folder", "i"), "Open folder"),
        el("button", { class: "btn quiet", type: "button", text: "Close", onclick: () => closePreview(s.id) }))));
}

function closePreview(id) {
  previewId = null;
  paintAll();
  (rows.get(id) || madeRows.get(id))?.querySelector("button.stamp, button.row")?.focus();
}

// ---------- Made before: the return log ----------
// One line per clip, newest first. A line with files to play opens the preview;
// one without (failed, or its files moved) opens its message instead.
const playableLine = (s) => !!s.status?.files?.length && !s.filesMissing;
const lineOpen = (s) => (playableLine(s) ? s.id === previewId : expanded.has(s.id));

function paintMade(visible, total) {
  const list = $("list-made");
  const keep = new Set(visible.map((s) => s.id));
  for (const [id, li] of madeRows) if (!keep.has(id)) { li.remove(); madeRows.delete(id); }
  let prev = null;
  for (const s of visible) {
    let li = madeRows.get(s.id);
    if (!li) { madeRows.set(s.id, (li = newLine(s.id))); li.dataset.id = s.id; }
    paintLine(li, s);
    const want = prev ? prev.nextSibling : list.firstChild;
    if (want !== li) list.insertBefore(li, want);
    prev = li;
  }
  $("more-made").hidden = visible.length >= total;
}

function newLine(id) {
  return el("li", {},
    el("button", { class: "row", type: "button", "aria-controls": "made-" + id, onclick: () => toggleLine(id) },
      el("div", { class: "thumb" }),
      el("span", { class: "what" }, el("span", { class: "t" }), el("span", { class: "m" })),
      el("span", { class: "stamp" })),
    el("div", { class: "detail", id: "made-" + id }));
}

function toggleLine(id) {
  const s = slips.find((x) => x.id === id);
  if (!s) return;
  if (playableLine(s)) previewId = previewId === id ? null : id;
  else if (!expanded.delete(id)) expanded.add(id);
  render();
}

function paintLine(li, s) {
  const title = titleOf(s);
  const stamp = S.stampFor(s);
  const files = s.status?.files || [];
  const open = lineOpen(s);
  li.querySelector(".t").textContent = title;
  const formats = (s.formats?.length ? s.formats : files.map((f) => f.format)).map((f) => FMT[f] || f.toUpperCase()).join(" + ");
  const when = s.madeAt ? new Date(s.madeAt) : null;
  const day = when && !isNaN(when) ? when.toLocaleDateString(undefined, { weekday: "short", month: "short", day: "numeric" }) : "";
  li.querySelector(".m").textContent = [formats, QUALITY[s.quality || settings.quality], day].filter(Boolean).join(" · ");
  const st = li.querySelector(".stamp");
  st.hidden = !stamp;
  st.textContent = stamp?.label || "";
  st.classList.toggle("done", stamp?.tone === "good");
  st.classList.toggle("bad", stamp?.tone === "bad");

  const failed = s.status?.kind === "failed";
  const thumbKey = failed + "|" + s.thumbnailUrl;
  if (li._thumb !== thumbKey) {
    li._thumb = thumbKey;
    const t = li.querySelector(".thumb");
    t.replaceChildren();
    if (failed || !isHttps(s.thumbnailUrl)) t.append(icon("scene-none"));
    else {
      const img = el("img", { src: s.thumbnailUrl, alt: "", loading: "lazy", referrerpolicy: "no-referrer" });
      img.addEventListener("error", () => img.replaceWith(icon("scene-none")));
      t.append(img);
    }
  }

  const row = li.querySelector("button.row");
  row.setAttribute("aria-expanded", String(open));
  const detail = li.querySelector(".detail");
  detail.hidden = !open;
  const key = JSON.stringify([s.status?.kind, s.status?.message, s.filesMissing, files.length]);
  if (li._detail !== key) {
    li._detail = key;
    const failedOrPartly = s.status?.message && s.status.kind !== "done" && s.status.kind !== "already";
    detail.replaceChildren(...[
      failedOrPartly ? el("p", { class: "note alert", text: s.status.message }) : null,
      files.length && s.filesMissing ? el("p", { class: "note", text: "File moved or deleted." }) : null,
      el("div", { class: "actions" },
        el("button", { class: "btn", type: "button", text: "Make again", "aria-label": "Make again, " + title, onclick: () => makeAgain(s.id) }),
        el("button", { class: "btn quiet", type: "button", text: "Remove", "aria-label": "Remove " + title, onclick: () => remove(s.id) })),
    ].filter(Boolean));
  } else {
    // titles can change on a re-lookup; keep the names in step without rebuilding
    const [again, rm] = detail.querySelectorAll("button");
    again.setAttribute("aria-label", "Make again, " + title);
    rm.setAttribute("aria-label", "Remove " + title);
  }
  paintPreview(li, s, s.id === previewId);
}

// makeAgain puts a fresh copy of the made clip in Later and saves it; the log
// line stays as the record. Focus moves to the new clip's Start box.
function makeAgain(id) {
  const copy = crypto.randomUUID();
  const next = S.makeAgain(slips, id, copy);
  if (next === slips) return;
  slips = next;
  persist(copy);
  focusStart = copy;
  render();
}

// ---------- actions ----------
async function validate(id, field) {
  const s = slips.find((x) => x.id === id);
  if (!s) return;
  const { startText, endText, endKind } = s;
  const r = await App.Validate(startText, endText, endKind === "length", s.durationSec);
  slips = slips.map((x) => {
    if (x.id !== id || x.startText !== startText || x.endText !== endText || x.endKind !== endKind) return x; // gone or retyped
    const changed = (x.checked ?? "|") !== S.typed(x);
    const keep = !changed || !x.status || ["queued", "making"].includes(x.status.kind);
    return {
      ...x, start: r.message ? null : r.start, end: r.message ? 0 : r.end, timeMsg: r.message,
      badField: r.message === x.timeMsg ? x.badField : field, checked: S.typed(x), status: keep ? x.status : null,
    };
  });
  persist(id);
  render();
}

// ---------- saving ----------
// toClip is the saved shape of a slip: only the user's own choices and the
// lookup. Results (status, files) are Go's to keep.
const CLIP_FIELDS = ["id", "lane", "url", "title", "thumbnailUrl", "durationSec", "source", "unavailable", "reason",
  "startText", "endText", "endKind", "start", "end", "timeMsg", "checked", "formats", "quality"];
const toClip = (s) => Object.fromEntries(CLIP_FIELDS.map((k) => [k, s[k] ?? null]));

// Every save and remove is queued behind the one before, so Go sees them in the
// order they happened, and Make can wait for all of them: it only makes slips
// whose saved lane is ready. A failure shows the notice and never throws.
let saving = Promise.resolve();
const queueSave = (call) => (saving = saving.then(call).catch((err) => notice(String(err?.message || err))));
// Placeholders and failed lookups aren't saved (Go keeps a found video itself); neither is a made slip.
function persist(...ids) {
  const clips = slips.filter((s) => ids.includes(s.id) && s.lane !== "made" && !s.pending && !s.failed && !isBusy(s)).map(toClip);
  if (clips.length) queueSave(() => App.Save(clips));
}

function remove(id) {
  if (isBusy(slips.find((s) => s.id === id))) return;
  slips = slips.filter((s) => s.id !== id);
  previewTab.delete(id);
  expanded.delete(id);
  if (previewId === id) previewId = null;
  queueSave(() => App.Remove(id));
  render();
  linkField.focus();
}

// retry looks a failed link up again, in the slip's place (e.g. after the video
// was made public). It runs through the same Add and slips:replace as a paste.
const retrying = new Set();
async function retry(id) {
  const s = slips.find((x) => x.id === id);
  if (!s || !S.canRetry(s) || retrying.has(id)) return;
  retrying.add(id);
  try {
    const [p] = (await App.Add(s.url)) || [];
    if (!p) {
      notice("That doesn't look like a video link any more. Remove it and paste it again.");
      return;
    }
    slips = S.retrySlip(slips, id, p);
    if (!slips.some((x) => x.id === p.id)) { // removed while we waited
      const early = buffered.get(p.id);
      buffered.delete(p.id);
      if (early) discardReplace(early);
      else dropped.add(p.id);
    } else if (buffered.has(p.id)) { // an instant failure beat Add's return
      applyReplace(buffered.get(p.id));
      buffered.delete(p.id);
    }
    notice("");
    render();
    linkField.focus();
  } finally {
    retrying.delete(id);
  }
}

// A lookup finished for a placeholder the page already let go of (removed, or
// Clear cache). Go saves what it finds before it replies, so take it back out.
function discardReplace(r) {
  for (const s of r.slips || []) queueSave(() => App.Remove(s.id));
}

function applyReplace(r) {
  const fresh = (r.slips || []).map((n) => ({ ...n, source: r.source }));
  slips = S.replaceSlip(slips, r.id, fresh);
}

async function add(text) {
  if (!text.trim()) return false;
  const placeholders = (await App.Add(text)) || [];
  if (!placeholders.length) {
    notice("That doesn't look like a video link. Copy it from the browser's address bar.");
    return false;
  }
  slips = S.addPlaceholders(slips, placeholders, settings); // the panel's choices, as of now
  for (const p of placeholders) {
    if (buffered.has(p.id)) { // an instant failure beat Add's return
      applyReplace(buffered.get(p.id));
      buffered.delete(p.id);
    }
  }
  notice("");
  render();
  return true;
}

// ---------- settings ----------
function showSettings() {
  document.documentElement.dataset.theme = settings.theme;
  for (const r of document.querySelectorAll('input[name="theme"]')) r.checked = r.value === settings.theme;
  for (const r of document.querySelectorAll('input[name="fullSource"]')) r.checked = r.value === String(settings.fullSource);
  for (const b of document.querySelectorAll("#formats .chip")) b.setAttribute("aria-pressed", String(settings.formats.includes(b.dataset.format)));
  for (const b of document.querySelectorAll("#quality button")) b.setAttribute("aria-pressed", String(b.dataset.quality === settings.quality));
  $("outdir").textContent = settings.outDir;
  $("outdir").title = settings.outDir;
  $("debug").checked = settings.debugMode;
  $("bindir").textContent = binDir;
  $("bindir").title = binDir;
}

async function save(next) {
  const prev = settings;
  settings = next;
  showSettings();
  render();
  try {
    await App.SaveSettings(next);
  } catch (err) {
    settings = prev;
    showSettings();
    render();
    notice(String(err?.message || err));
  }
}

$("formats").addEventListener("click", (e) => {
  const b = e.target.closest(".chip");
  if (!b) return;
  const f = b.dataset.format;
  const formats = S.toggleFormat(settings.formats, f);
  if (formats === settings.formats) return notice("At least one format stays on.");
  save({ ...settings, formats });
});
$("quality").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (b) save({ ...settings, quality: b.dataset.quality });
});
$("change-folder").addEventListener("click", async () => {
  const dir = await App.PickFolder();
  if (dir) save({ ...settings, outDir: dir });
});
for (const r of document.querySelectorAll('input[name="theme"]')) r.addEventListener("change", () => save({ ...settings, theme: r.value }));
for (const r of document.querySelectorAll('input[name="fullSource"]')) r.addEventListener("change", () => save({ ...settings, fullSource: r.value === "true" }));
$("debug").addEventListener("change", (e) => save({ ...settings, debugMode: e.target.checked }));
$("open-bin").addEventListener("click", () => App.OpenFolder(binDir));
$("open-logs").addEventListener("click", () => App.OpenLogs());
$("settings").addEventListener("toggle", (e) => {
  $("gear").setAttribute("aria-expanded", String(e.newState === "open"));
  if (e.newState !== "open") { $("cache-confirm").hidden = true; $("clear-cache").hidden = false; } // the question doesn't wait for next time
});

// ---------- adding links: the field, paste anywhere, drop ----------
$("paste").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (await add(linkField.value)) linkField.value = "";
});
document.addEventListener("paste", (e) => {
  if (!$("setup").hidden || e.target.closest?.("input, textarea, [contenteditable]")) return;
  const text = e.clipboardData?.getData("text") || "";
  if (text.trim()) { e.preventDefault(); add(text); }
});
const drop = $("drop");
// Every drag is cancelled at the window so a dropped file never navigates the page.
window.addEventListener("dragover", (e) => {
  e.preventDefault();
  drop.classList.toggle("over", !!e.target.closest?.("#drop"));
});
window.addEventListener("dragleave", (e) => { if (!e.relatedTarget) drop.classList.remove("over"); });
window.addEventListener("drop", (e) => {
  e.preventDefault();
  drop.classList.remove("over");
  if (!$("setup").hidden) return;
  const uris = (e.dataTransfer.getData("text/uri-list") || "").split(/\r?\n/).filter((l) => l && !l.startsWith("#")).join("\n");
  const text = uris || e.dataTransfer.getData("text/plain");
  if (text.trim()) add(text);
  else notice("Drop a video link here, not a file.");
});

// ---------- making ----------
let starting = false;
makeBtn.addEventListener("click", async () => {
  if (running) return App.Stop();
  if (starting) return;
  starting = true;
  try {
    // typed-but-unchecked times are validated first, so jobs carry what was typed
    await Promise.all(S.needsCheck(slips).map((id) => validate(id, slips.find((x) => x.id === id)?.badField || "start")));
    await saving; // Go makes only what it has saved as ready
  } finally {
    starting = false; // a rejected Validate must not leave the button dead
  }
  const jobs = S.readyJobs(slips);
  if (!jobs.length) return;
  running = true;
  notice("");
  slips = S.startRun(slips, jobs.map((j) => j.id));
  render();
  App.Make(jobs);
});

// the button disables itself once nothing is left to move, so focus goes to the
// Ready heading; the counts show what moved
$("mark-all").addEventListener("click", async () => {
  const moved = await markReady(S.inLane(slips, "later").map((s) => s.id));
  if (moved.length) $("h-ready").focus();
});

// Show more reveals the rest of the log at once and goes away, so focus moves to
// the first line it added
$("more-made").addEventListener("click", () => {
  const shown = S.madeVisible(slips, false).length;
  showAllMade = true;
  paintAll();
  const added = S.madeVisible(slips, true)[shown];
  madeRows.get(added?.id)?.querySelector("button.row")?.focus();
});

// Clear made forgets the history only; no files are touched, so it doesn't ask.
$("clear-made").addEventListener("click", async () => {
  try {
    await App.ClearMade();
  } catch (err) {
    return notice(String(err?.message || err));
  }
  slips = slips.filter((s) => s.lane !== "made");
  expanded.clear();
  previewId = slips.some((s) => s.id === previewId) ? previewId : null;
  showAllMade = false;
  render();
  linkField.focus();
});

// Clear cache asks once, in place, because it throws away queued work too.
const cacheAsk = (asking) => {
  $("cache-confirm").hidden = !asking;
  $("clear-cache").hidden = asking;
  $(asking ? "cache-no" : "clear-cache").focus();
};
$("clear-cache").addEventListener("click", () => cacheAsk(true));
$("cache-no").addEventListener("click", () => cacheAsk(false));
$("cache-yes").addEventListener("click", async () => {
  try {
    await saving; // a save still queued would land after the wipe and bring rows back
    await App.ClearCache();
  } catch (err) {
    cacheAsk(false);
    return notice(String(err?.message || err));
  }
  // Lookups still in flight will reply to rows that are gone: drop those replies.
  for (const s of slips) if (s.pending) dropped.add(s.id);
  for (const r of buffered.values()) discardReplace(r);
  buffered.clear();
  slips = [];
  previewId = null;
  expanded.clear();
  previewTab.clear();
  showAllMade = false;
  notice("");
  cacheAsk(false);
  render();
  $("settings").hidePopover?.(); // so the empty shelf shows; focus goes back to the gear
});

on("shelf:notice",(m) => notice(m?.message || ""));
on("slips:replace", (r) => {
  if (dropped.delete(r.id)) return discardReplace(r);
  if (!slips.some((s) => s.id === r.id)) { buffered.set(r.id, r); return; }
  applyReplace(r);
  render();
});
on("slip:status", (e) => { slips = S.applyStatus(slips, e); render(); });
on("run:finished", () => { running = false; slips = S.endRun(slips); render(); });
on("run:busy", (m) => notice(m?.message || ""));

// ---------- first launch ----------
let setupAcc;
const setupBar = $("setup-progress");
const READY_H = $("setup-h").textContent;
// setupFailed swaps the screen to its error state: the downloading text and
// bar go, so it never says "downloading" above "couldn't download"
function setupFailed(message, needsAdmin) {
  $("setup-h").textContent = "Couldn't finish getting ready";
  $("setup-text").hidden = true;
  setupBar.hidden = true;
  $("setup-error").textContent = message;
  $("setup-error").hidden = false;
  $("setup-retry").hidden = needsAdmin;
}
function showSetup(needsAdmin) {
  document.getElementById("settings").hidePopover?.();
  $("setup").hidden = false;
  appEl.inert = true;
  $("setup-h").textContent = READY_H;
  $("setup-text").hidden = false;
  setupBar.hidden = false;
  $("setup-retry").hidden = true;
  $("setup-error").hidden = true;
  if (needsAdmin) setupFailed(ADMIN, true);
  $("setup-h").focus();
  if (needsAdmin) return;
  setupAcc = undefined;
  setBar(setupBar, 0, "Starting the download…", true);
  App.Install();
}
on("setup", (e) => {
  setupAcc = S.setupProgress(setupAcc, e);
  const v = S.barValue(setupAcc.pct);
  // an unmeasured download (no size) shows no percentage and the sweep
  if (setupAcc.known) setBar(setupBar, setupAcc.pct, `Downloading ${Math.round(v * 100)}%`, false);
  else setBar(setupBar, -1, "Downloading…", false);
});
on("setup:done", () => {
  $("setup").hidden = true;
  appEl.inert = false;
  linkField.focus();
});
on("setup:error", (e) => {
  setupFailed(e.message, e.needsAdmin);
  if (e.needsAdmin) $("setup-h").focus();
  else $("setup-retry").focus();
});
on("setup:needed", () => { if ($("setup").hidden) showSetup(false); });
$("setup-retry").addEventListener("click", () => showSetup(false));

// ---------- start ----------
// Settings answers at once: apply the theme before Boot's tool checks (about
// half a second) so a mint or lilac window never flashes grey.
settings = await App.Settings();
showSettings();
const boot = await App.Boot();
settings = boot.settings;
binDir = boot.binDir;
slips = S.fromShelf(boot.shelf?.clips || []);
if (boot.shelf?.notice) notice(boot.shelf.notice);
showSettings();
render();
if (boot.needsSetup) showSetup(boot.needsAdmin);
else linkField.focus();
