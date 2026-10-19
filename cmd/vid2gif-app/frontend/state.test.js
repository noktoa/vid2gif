// Run: node --test "cmd/vid2gif-app/frontend/*.test.js"
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addPlaceholders, replaceSlip, readyJobs, applyStatus, eyeId, makeLabel,
  formatDuration, stageLabel, barValue, startRun, endRun, sourceTitle,
  setupProgress, formatSize, clipLength,
} from "./state.js";

const slip = (id, more = {}) => ({
  id, url: "https://v.example/" + id, title: "T " + id, thumbnailUrl: "", durationSec: 300,
  unavailable: false, reason: "", failed: false, message: "",
  startText: "", endText: "", start: null, end: 0, timeMsg: "", status: null, ...more,
});
const ready = (id, more) => slip(id, { startText: "0:10", start: 10, ...more });
const ev = (jobId, kind, more = {}) => ({ jobId, kind, stage: "", message: "", pct: -1, stagePct: -1, files: [], ...more });

test("addPlaceholders appends placeholders and ignores links already on the shelf", () => {
  const a = addPlaceholders([], [{ id: "p1", url: "https://y/1" }, { id: "p2", url: "https://y/1" }]);
  assert.deepEqual(a.map((s) => s.id), ["p1"]);
  assert.equal(a[0].pending, true);
  const b = addPlaceholders(a, [{ id: "p3", url: "https://y/1" }, { id: "p4", url: "https://y/2" }]);
  assert.deepEqual(b.map((s) => s.id), ["p1", "p4"]);
});

test("replaceSlip expands one placeholder into many in place", () => {
  const s = addPlaceholders([ready("a")], [{ id: "p", url: "https://y/list" }]).concat([ready("z")]);
  const out = replaceSlip(s, "p", [slip("x"), slip("y")]);
  assert.deepEqual(out.map((v) => v.id), ["a", "x", "y", "z"]);
  assert.equal(out[1].pending, undefined);
  assert.equal(out[1].start, null);
});

test("replaceSlip leaves the list alone for an id it does not have", () => {
  const s = [ready("a")];
  assert.equal(replaceSlip(s, "nope", [slip("x")]), s);
});

test("replaceSlip drops videos already on the shelf, keeps unavailable entries without a link", () => {
  const s = addPlaceholders([ready("a")], [{ id: "p", url: "https://y/list" }]);
  const out = replaceSlip(s, "p", [slip("dup", { url: s[0].url }), slip("n1", { url: "", unavailable: true }), slip("n2", { url: "", unavailable: true })]);
  assert.deepEqual(out.map((v) => v.id), ["a", "n1", "n2"]);
});

test("readyJobs skips slips without a Start, unavailable, failed, pending and finished ones", () => {
  const s = [
    ready("ok", { endText: "0:20", end: 20 }),
    slip("nostart"),
    ready("gone", { unavailable: true }),
    ready("bad", { failed: true }),
    ready("ph", { pending: true }),
    ready("err", { timeMsg: "Stop must be after Start." }),
    ready("done", { status: { kind: "done", files: [] } }),
    ready("retry", { status: { kind: "failed", message: "x", files: [] } }),
  ];
  assert.deepEqual(readyJobs(s), [
    { id: "ok", url: s[0].url, title: "T ok", start: 10, end: 20, durationSec: 300 },
    { id: "retry", url: s[7].url, title: "T retry", start: 10, end: 0, durationSec: 300 },
  ]);
});

test("applyStatus: making keeps the stage and never lowers progress", () => {
  let s = [ready("a")];
  s = applyStatus(s, ev("a", "making", { stage: "download", pct: 0.4, stagePct: 0.63 }));
  assert.deepEqual(s[0].status, { kind: "making", stage: "download", pct: 0.4, stagePct: 0.63, message: "", files: [] });
  s = applyStatus(s, ev("a", "making", { stage: "mp4", pct: 0.38, stagePct: 0.1 }));
  assert.equal(s[0].status.pct, 0.4);
  assert.equal(s[0].status.stage, "mp4");
  s = applyStatus(s, ev("a", "making", { stage: "mp4", pct: 0.5, stagePct: 0.2 }));
  assert.equal(s[0].status.pct, 0.5);
});

test("applyStatus: making with unknown length stays indeterminate", () => {
  const s = applyStatus([ready("a")], ev("a", "making", { stage: "find", pct: -1 }));
  assert.equal(s[0].status.pct, -1);
});

test("applyStatus: done, partial, already, failed keep files and message; null files are []", () => {
  const f = [{ format: "gif", path: "C:\\o\\a.gif", size: 10, url: "/out/a.gif" }];
  for (const kind of ["done", "partial", "already"]) {
    const s = applyStatus([ready("a")], ev("a", kind, { files: f, message: kind === "partial" ? "The MP4 couldn't be made." : "" }));
    assert.equal(s[0].status.kind, kind);
    assert.deepEqual(s[0].status.files, f);
  }
  const s = applyStatus([ready("a")], ev("a", "failed", { files: null, message: "Couldn't reach the internet. Check your connection and try again." }));
  assert.equal(s[0].status.kind, "failed");
  assert.deepEqual(s[0].status.files, []);
  assert.match(s[0].status.message, /internet/);
});

test("applyStatus: ready clears the status (cancelled, back to waiting)", () => {
  let s = applyStatus([ready("a")], ev("a", "making", { stage: "gif", pct: 0.5 }));
  s = applyStatus(s, ev("a", "ready"));
  assert.equal(s[0].status, null);
});

test("applyStatus ignores an unknown id without throwing", () => {
  const s = [ready("a")];
  assert.equal(applyStatus(s, ev("ghost", "done")), s);
});

test("startRun marks jobs as next up; endRun returns unfinished ones to ready", () => {
  let s = startRun([ready("a"), ready("b"), ready("c", { status: { kind: "done", files: [] } })], ["a", "b"]);
  assert.deepEqual(s.map((v) => v.status?.kind ?? null), ["queued", "queued", "done"]);
  s = applyStatus(s, ev("a", "making", { stage: "gif", pct: 0.5 }));
  s = endRun(s);
  assert.deepEqual(s.map((v) => v.status?.kind ?? null), [null, null, "done"]);
});

test("eyeId returns exactly one id or null, never two", () => {
  const f = [{ format: "gif", path: "p", size: 1, url: "/out/x" }];
  const s = [
    ready("done", { status: { kind: "done", files: f } }),
    ready("make", { status: { kind: "making", stage: "gif", pct: 0.5, stagePct: -1, files: [] } }),
  ];
  assert.equal(eyeId(s, "done"), "done");
  assert.equal(eyeId(s, null), "make");
  assert.equal(eyeId(s, "removed"), "make");
  assert.equal(eyeId([ready("x")], null), null);
  assert.equal(typeof eyeId(s, "done"), "string");
});

test("makeLabel", () => {
  assert.equal(makeLabel(0), "Make clips");
  assert.equal(makeLabel(1), "Make 1 clip");
  assert.equal(makeLabel(3), "Make 3 clips");
});

test("formatDuration", () => {
  assert.equal(formatDuration(272), "4:32");
  assert.equal(formatDuration(0), "—");
  assert.equal(formatDuration(58), "0:58");
  assert.equal(formatDuration(3912), "1:05:12");
  assert.equal(formatDuration(7.9), "0:07");
});

test("stageLabel", () => {
  assert.equal(stageLabel({ stage: "find", stagePct: -1 }), "Finding the video…");
  assert.equal(stageLabel({ stage: "download", stagePct: 0.63 }), "Getting the clip 63%");
  assert.equal(stageLabel({ stage: "gif", stagePct: -1 }), "Making GIF…");
  assert.equal(stageLabel({ stage: "mp4", stagePct: 0.4 }), "Making MP4 40%");
  assert.equal(stageLabel({ stage: "webp", stagePct: -1 }), "Making WebP…");
  assert.equal(stageLabel({ stage: "mov", stagePct: 0.29 }), "Making MOV 29%");
});

test("stageLabel never shows a percentage when unmeasured, and never 100%", () => {
  for (const stage of ["find", "download", "gif", "webp", "mp4", "mov"]) {
    assert.doesNotMatch(stageLabel({ stage, stagePct: -1 }), /%|\d\s*$/);
    assert.doesNotMatch(stageLabel({ stage, stagePct: 1 }), /100%/);
    assert.doesNotMatch(stageLabel({ stage, stagePct: 0.999 }), /100%/);
  }
});

test("barValue: one value for the bar, null when indeterminate, never full before done", () => {
  assert.equal(barValue(-1), null);
  assert.equal(barValue(0.41), 0.41);
  assert.equal(barValue(1), 0.99);
  assert.ok(Math.round(barValue(0.999) * 100) < 100);
});

test("sourceTitle names the sources on the shelf", () => {
  assert.equal(sourceTitle([]), "");
  assert.equal(sourceTitle([slip("a", { source: "Pasted links" })]), "Pasted links");
  assert.equal(sourceTitle([slip("a", { source: "Family videos" }), slip("b", { source: "Pasted links" }), slip("c", { source: "Family videos" })]),
    "Family videos, plus pasted links");
});

test("setupProgress moves one bar across the tools and never goes back", () => {
  let p = setupProgress(undefined, { tool: "deno", done: 50, total: 100 });
  assert.ok(Math.abs(p.pct - 1 / 6) < 1e-9);
  p = setupProgress(p, { tool: "ffmpeg", done: 0, total: 1000 });
  assert.ok(Math.abs(p.pct - 1 / 3) < 1e-9);
  const before = p.pct;
  p = setupProgress(p, { tool: "ffmpeg", done: 10, total: -1 });
  assert.equal(p.pct, before);
  p = setupProgress(p, { tool: "yt-dlp", done: 100, total: 100 });
  assert.ok(p.pct < 1);
});

test("formatSize and clipLength", () => {
  assert.equal(formatSize(2516582), "2.4 MB");
  assert.equal(formatSize(850 * 1024), "850 KB");
  assert.equal(clipLength(ready("a", { end: 17 })), 7);
  assert.equal(clipLength(ready("a", { durationSec: 70 })), 60);
  assert.equal(clipLength(ready("a", { durationSec: 0 })), 0);
});
