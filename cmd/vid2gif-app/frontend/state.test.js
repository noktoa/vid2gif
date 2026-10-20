// Run: node --test "cmd/vid2gif-app/frontend/*.test.js"
import { test } from "node:test";
import assert from "node:assert/strict";
import {
  addPlaceholders, replaceSlip, readyJobs, applyStatus, eyeId, makeLabel,
  formatDuration, stageLabel, barValue, needsCheck, makeCount, startRun, endRun, sourceTitle,
  setupProgress, formatSize, clipLength, canRetry, retrySlip, setEndKind, typed,
  toggleFormat, setOptions, fromShelf, inLane, madeVisible, markReady, markAllReady, moveToLater, makeAgain, stampFor,
} from "./state.js";

const slip = (id, more = {}) => ({
  id, lane: "ready", url: "https://v.example/" + id, title: "T " + id, thumbnailUrl: "", durationSec: 300,
  unavailable: false, reason: "", failed: false, message: "",
  startText: "", endText: "", start: null, end: 0, timeMsg: "", checked: "|", status: null, ...more,
});
const ready = (id, more = {}) => {
  const s = slip(id, { startText: "0:10", start: 10, ...more });
  return { ...s, checked: "checked" in more ? more.checked : s.startText + "|" + s.endText };
};
const ev = (jobId, kind, more = {}) => ({ jobId, kind, stage: "", message: "", pct: -1, stagePct: -1, files: [], ...more });

test("addPlaceholders appends placeholders, and the same link can be added again for another clip", () => {
  const a = addPlaceholders([], [{ id: "p1", url: "https://y/1" }, { id: "p2", url: "https://y/1" }]);
  assert.deepEqual(a.map((s) => s.id), ["p1", "p2"]);
  assert.equal(a[0].pending, true);
  const b = addPlaceholders(a, [{ id: "p3", url: "https://y/1" }, { id: "p4", url: "https://y/2" }]);
  assert.deepEqual(b.map((s) => s.id), ["p1", "p2", "p3", "p4"]);
});

test("a link starts from the panel's choices, keeps its own after, and a playlist's entries and a retry inherit them", () => {
  const panel = { formats: ["gif"], quality: "medium" };
  let s = addPlaceholders([], [{ id: "p1", url: "https://y/list" }], panel);
  panel.formats.push("mp4"); // the panel changing later must not reach the slip
  assert.deepEqual(s[0].formats, ["gif"]);
  s = setOptions(s, "p1", { formats: ["webp"], quality: "high" });
  s = replaceSlip(s, "p1", [slip("x"), slip("y")]);
  assert.deepEqual(s.map((x) => [x.formats, x.quality]), [[["webp"], "high"], [["webp"], "high"]]);
  s = addPlaceholders(s, [{ id: "p2", url: "https://y/2" }], { formats: ["mov"], quality: "low" }); // later link: new defaults
  assert.deepEqual([s[2].formats, s[2].quality], [["mov"], "low"]);
  const bad = replaceSlip(s, "p2", [slip("bad", { failed: true, url: "https://y/2" })]);
  const again = retrySlip(bad, "bad", { id: "p3", url: "https://y/2" });
  assert.deepEqual([again[2].formats, again[2].quality], [["mov"], "low"]);
});

test("readyJobs sends each slip's own formats and quality", () => {
  const jobs = readyJobs([ready("a", { formats: ["mp4"], quality: "high" }), ready("b", { formats: ["gif", "webp"], quality: "low" })]);
  assert.deepEqual(jobs.map((j) => [j.formats, j.quality]), [[["mp4"], "high"], [["gif", "webp"], "low"]]);
});

test("toggleFormat keeps the fixed order and never turns the last format off", () => {
  assert.deepEqual(toggleFormat(["gif", "mp4"], "webp"), ["gif", "webp", "mp4"]);
  assert.deepEqual(toggleFormat(["gif", "webp"], "gif"), ["webp"]);
  const one = ["gif"];
  assert.equal(toggleFormat(one, "gif"), one);
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

test("replaceSlip keeps a video that is already on the shelf, and unavailable entries without a link", () => {
  const s = addPlaceholders([ready("a")], [{ id: "p", url: "https://y/list" }]);
  const out = replaceSlip(s, "p", [slip("dup", { url: s[0].url }), slip("n1", { url: "", unavailable: true }), slip("n2", { url: "", unavailable: true })]);
  assert.deepEqual(out.map((v) => v.id), ["a", "dup", "n1", "n2"]);
});

test("canRetry is true only for a failed lookup that has a link", () => {
  assert.equal(canRetry(slip("f", { failed: true, message: "Couldn't get this video. It may be private or removed." })), true);
  assert.equal(canRetry(slip("e", { failed: true, url: "" })), false); // nothing to look up again
  assert.equal(canRetry(slip("u", { url: "", unavailable: true })), false); // a private entry inside a playlist
  assert.equal(canRetry(ready("r")), false);
  assert.equal(canRetry({ ...slip("p"), pending: true }), false);
});

test("retrySlip puts a fresh placeholder where the failed slip was", () => {
  const s = [ready("a"), slip("bad", { failed: true, message: "Couldn't get this video. It may be private or removed." }), ready("z")];
  const out = retrySlip(s, "bad", { id: "p9", url: s[1].url });
  assert.deepEqual(out.map((v) => v.id), ["a", "p9", "z"]);
  assert.equal(out[1].pending, true);
  assert.ok(!out[1].failed && !out[1].message); // nothing of the old failure is carried over
  assert.equal(out[1].url, s[1].url);
  assert.equal(out[0], s[0]); // the others are untouched
});

test("retrySlip leaves the list alone for an unknown id or a slip that cannot be retried", () => {
  const s = [ready("a"), slip("n", { url: "", unavailable: true })];
  assert.equal(retrySlip(s, "nope", { id: "p", url: "https://y/1" }), s);
  assert.equal(retrySlip(s, "a", { id: "p", url: "https://y/1" }), s); // a good video is not retried
  assert.equal(retrySlip(s, "n", { id: "p", url: "https://y/1" }), s);
});

test("retrySlip result takes the usual replace: a successful lookup fills the placeholder", () => {
  const s = [slip("bad", { failed: true })];
  const pending = retrySlip(s, "bad", { id: "p9", url: s[0].url });
  const done = replaceSlip(pending, "p9", [slip("good", { url: s[0].url })]);
  assert.deepEqual(done.map((v) => v.id), ["good"]);
  assert.equal(done[0].failed, false);
});

test("readyJobs skips slips without a Start, unavailable, failed lookups, placeholders and bad times", () => {
  const s = [
    ready("ok", { endText: "0:20", end: 20, formats: ["gif"], quality: "medium" }),
    slip("nostart"),
    ready("gone", { unavailable: true }),
    ready("bad", { failed: true }),
    ready("ph", { pending: true }),
    ready("err", { timeMsg: "Stop must be after Start." }),
  ];
  assert.deepEqual(readyJobs(s), [{ id: "ok", url: s[0].url, title: "T ok", start: 10, end: 20, durationSec: 300, formats: ["gif"], quality: "medium" }]);
});

test("readyJobs includes finished slips again: done, partial, already and run-failed", () => {
  const f = [{ format: "gif", path: "p", size: 1, url: "/out/x" }];
  const s = [
    ready("done", { status: { kind: "done", files: f } }),
    ready("partial", { status: { kind: "partial", message: "The MP4 couldn't be made.", files: f } }),
    ready("already", { status: { kind: "already", files: f } }),
    ready("retry", { status: { kind: "failed", message: "x", files: [] } }),
  ];
  assert.deepEqual(readyJobs(s).map((j) => j.id), ["done", "partial", "already", "retry"]);
});

test("readyJobs leaves out slips in the current run", () => {
  const s = [ready("q", { status: { kind: "queued", files: [] } }), ready("m", { status: { kind: "making", stage: "gif", pct: 0.5, stagePct: -1, files: [] } })];
  assert.deepEqual(readyJobs(s), []);
});

test("readyJobs leaves out a slip whose text changed since it was validated; needsCheck names it", () => {
  const s = [ready("a"), ready("edited", { startText: "0:20", checked: "0:10|" }), slip("fresh"), ready("gone", { startText: "1", checked: "", unavailable: true })];
  assert.deepEqual(readyJobs(s).map((j) => j.id), ["a"]);
  assert.deepEqual(needsCheck(s), ["edited"]);
  assert.equal(makeCount(s), 2);
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

test("applyStatus: a new run replaces the previous run's files", () => {
  const old = [{ format: "gif", path: "a_medium.gif", size: 1, url: "/out/a" }];
  const neu = [{ format: "gif", path: "a_high.gif", size: 2, url: "/out/b" }];
  let s = [ready("a", { status: { kind: "done", files: old } })];
  s = startRun(s, ["a"]);
  assert.deepEqual(s[0].status.files, []);
  s = applyStatus(s, ev("a", "making", { stage: "find", pct: 0.01 }));
  assert.deepEqual(s[0].status.files, []);
  s = applyStatus(s, ev("a", "done", { pct: 1, files: neu }));
  assert.deepEqual(s[0].status.files, neu);
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

test("setEndKind rewrites a validated end to match, and clears one that is not validated", () => {
  const stop = ready("a", { startText: "1:00", start: 60, endText: "1:30", end: 90, checked: "1:00|1:30" });
  const asLength = setEndKind([stop], "a", "length")[0];
  assert.deepEqual([asLength.endKind, asLength.endText], ["length", "0:30"]);
  assert.notEqual(typed(asLength), stop.checked); // so it is validated again
  const back = setEndKind([{ ...asLength, checked: typed(asLength) }], "a", "stop")[0];
  assert.deepEqual([back.endKind, back.endText], ["stop", "1:30"]);
  const typing = ready("b", { startText: "1:00", endText: "0:30", checked: "|" });
  assert.equal(setEndKind([typing], "b", "length")[0].endText, "");
  const toEnd = ready("c", { startText: "1:00", start: 60, endText: "", end: 0, checked: "1:00|" });
  assert.equal(setEndKind([toEnd], "c", "length")[0].endText, "");
  const same = [stop];
  assert.equal(setEndKind(same, "a", "stop")[0], stop);
  assert.equal(setEndKind(same, "nope", "length"), same);
});

test("a length slip needs its own check, even with the same text", () => {
  const s = ready("a", { endText: "0:05", checked: "0:10|0:05", endKind: "length" });
  assert.deepEqual(needsCheck([s]), ["a"]);
});

const ids = (slips) => slips.map((s) => s.id);

test("readyJobs, needsCheck and makeCount only look at the Ready lane", () => {
  const s = [ready("r"), ready("l", { lane: "later" }), ready("m", { lane: "made" }), slip("l2", { lane: "later", startText: "5" }), slip("r2", { startText: "5" })];
  assert.deepEqual(ids(readyJobs(s)), ["r"]);
  assert.deepEqual(needsCheck(s), ["r2"]);
  assert.equal(makeCount(s), 2);
});

test("new links land in Later, and a replaced or retried slip keeps its lane", () => {
  let s = addPlaceholders([], [{ id: "p", url: "https://y/1" }]);
  assert.equal(s[0].lane, "later");
  s = replaceSlip([{ ...s[0], lane: "ready" }], "p", [slip("x", { lane: "later" }), slip("y", { lane: "later" })]);
  assert.deepEqual(s.map((v) => v.lane), ["ready", "ready"]);
  const bad = [slip("bad", { lane: "later", failed: true })];
  assert.equal(retrySlip(bad, "bad", { id: "p2", url: bad[0].url })[0].lane, "later");
});

test("markAllReady moves only the valid Later slips, in order, and leaves one with no Start in Later", () => {
  const s = [ready("r"), ready("a", { lane: "later" }), slip("nostart", { lane: "later" }), ready("b", { lane: "later" }), ready("m", { lane: "made" })];
  const out = markAllReady(s);
  assert.deepEqual(inLane(out, "ready").map((v) => v.id), ["r", "a", "b"]);
  assert.deepEqual(ids(inLane(out, "later")), ["nostart"]);
  assert.deepEqual(ids(inLane(out, "made")), ["m"]);
  assert.equal(s[1].lane, "later"); // input untouched
});

test("markReady moves only the named ids, and only the valid ones", () => {
  const s = [ready("a", { lane: "later" }), ready("b", { lane: "later" }), ready("stale", { lane: "later", checked: "x|" }), ready("m", { lane: "made" })];
  const out = markReady(s, ["b", "stale", "m"]);
  assert.deepEqual(ids(inLane(out, "ready")), ["b"]);
  assert.deepEqual(ids(inLane(out, "later")), ["a", "stale"]);
  assert.equal(inLane(out, "made")[0].id, "m");
});

test("moveToLater changes only the lane", () => {
  const s = [ready("a"), ready("b")];
  const out = moveToLater(s, "a");
  assert.deepEqual(out[0], { ...s[0], lane: "later" });
  assert.equal(out[1], s[1]);
  assert.equal(moveToLater(s, "nope"), s);
});

test("applyStatus puts a finished slip in Made, last, and a made slip cannot be run", () => {
  for (const kind of ["done", "partial", "failed", "already"]) {
    const out = applyStatus([ready("a"), ready("b")], ev("a", kind));
    assert.equal(out[1].lane, "made");
    assert.deepEqual(ids(out), ["b", "a"]);
  }
  const made = applyStatus([ready("a")], ev("a", "done"));
  assert.deepEqual(readyJobs(made), []);
  assert.deepEqual(needsCheck(made.map((v) => ({ ...v, checked: "" }))), []);
  assert.equal(applyStatus([ready("a")], ev("a", "making", { stage: "gif" }))[0].lane, "ready");
});

test("makeAgain appends a fresh copy to Later and leaves the made slip alone", () => {
  const f = [{ format: "gif", path: "p", size: 1, url: "/out/x" }];
  const made = ready("a", { lane: "made", resultKind: "done", filesMissing: true, formats: ["gif"], quality: "high", endText: "0:20", end: 20, status: { kind: "done", files: f } });
  const s = [made, ready("z")];
  const out = makeAgain(s, "a", "n1");
  assert.equal(out[0], made);
  assert.equal(out.length, 3);
  const c = out[2];
  assert.deepEqual([c.id, c.lane, c.status, c.resultKind, c.filesMissing], ["n1", "later", null, "", false]);
  assert.deepEqual([c.url, c.startText, c.endText, c.start, c.end, c.formats, c.quality, c.checked], [made.url, "0:10", "0:20", 10, 20, ["gif"], "high", made.checked]);
  assert.equal(makeAgain(s, "nope", "n2"), s);
});

test("madeVisible is newest first and shows 5 unless showAll", () => {
  const s = [ready("r"), ...["a", "b", "c", "d", "e", "f", "g"].map((i) => ready(i, { lane: "made" }))];
  assert.deepEqual(ids(madeVisible(s, false)), ["g", "f", "e", "d", "c"]);
  assert.deepEqual(ids(madeVisible(s, true)), ["g", "f", "e", "d", "c", "b", "a"]);
});

test("fromShelf rebuilds slips and a made clip keeps its files and what was missing", () => {
  const f = [{ format: "gif", path: "C:\o\a.gif", size: 10, url: "/out/a.gif" }];
  const saved = [
    { id: "m", lane: "made", url: "https://y/1", title: "T", startText: "0:10", endText: "", endKind: "stop", start: 10, end: 0, timeMsg: "", checked: "0:10|", formats: ["gif"], quality: "high",
      resultKind: "partial", resultMessage: "The MP4 couldn't be made.", files: f, filesMissing: true, createdAt: "2026-01-01T00:00:00Z", madeAt: "2026-01-02T00:00:00Z" },
    { id: "l", lane: "later", url: "https://y/2", title: "U", startText: "", start: null, formats: null, resultKind: "", files: null, filesMissing: false },
  ];
  const [m, l] = fromShelf(saved);
  assert.deepEqual(m.status, { kind: "partial", message: "The MP4 couldn't be made.", files: f, stage: "", pct: 1, stagePct: -1 });
  assert.deepEqual([m.lane, m.filesMissing, m.resultKind, m.formats, m.quality, m.start], ["made", true, "partial", ["gif"], "high", 10]);
  assert.deepEqual([l.lane, l.status, l.start, l.formats, l.pending, l.failed], ["later", null, null, [], undefined, undefined]);
  assert.equal(l.endKind, "stop"); // page-side defaults still apply
});

test("stampFor labels a made slip", () => {
  const k = (kind) => stampFor({ status: { kind } });
  assert.deepEqual(k("done"), { label: "DONE", tone: "good" });
  assert.deepEqual(k("already"), { label: "DONE", tone: "good" });
  assert.deepEqual(k("partial"), { label: "PARTLY", tone: "bad" });
  assert.deepEqual(k("failed"), { label: "FAILED", tone: "bad" });
});
