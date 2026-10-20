// Dev-only fake of the Go side, so the page opens in a plain browser.
// Loading contract: index.html document.write()s this script, synchronously,
// only when window.go is missing, so in the app it is never fetched. App
// scripts placed after that inline loader can rely on window.go and
// window.runtime existing when they run; app.js should still wait for
// window.go before its first call (a deferred/module script satisfies this).
// Shapes follow internal/appcore and internal/runner JSON tags exactly.
// index.html?setup makes Boot report that the helper files are missing;
// index.html?setup=fail also makes Install fail.
(() => {
  if (window.go) return; // the real bindings win
  const params = new URLSearchParams(location.search);
  const listeners = {};
  const emit = (name, data) => (listeners[name] || []).slice().forEach((cb) => cb(data));
  const later = (ms, fn) => setTimeout(fn, ms);
  let nextId = 100;
  const id = () => "s" + nextId++;

  let settings = {
    theme: "grey", outDir: "C:\\Users\\you\\Videos\\Clips", formats: ["gif", "mp4"],
    quality: "medium", fullSource: false, debugMode: false,
  };
  const binDir = "C:\\Users\\you\\AppData\\Local\\Programs\\vid2gif\\bin";

  // appcore.Slip samples: a playlist's entries, one unavailable.
  const playlist = [
    { title: "Grandma’s 90th birthday: the cake moment", durationSec: 272 },
    { title: "Best man toast, Sam and Jo’s wedding", durationSec: 728 },
    { title: "Skateboard tricks compilation 12", durationSec: 495 },
    { title: "Sunrise timelapse over Mt. Fuji", durationSec: 167 },
    { title: "Holiday dinner, the whole table singing", durationSec: 3912 },
    { title: "First steps in the garden", durationSec: 58 },
    { title: "", durationSec: 0, unavailable: true, reason: "Couldn’t get this video. It may be private or removed." },
  ];
  const slip = (url, v) => ({
    id: id(), url, title: v.title, thumbnailUrl: "", durationSec: v.durationSec,
    unavailable: !!v.unavailable, reason: v.reason || "", failed: false, message: "",
  });

  // appcore.Status samples, one per slip state (pct/stagePct -1 = unknown).
  const file = (format, size) => ({ format, path: `${settings.outDir}\\sample_1m12s-1m19s.${format}`, size, url: "" });
  const status = (jobId, kind, more = {}) => ({ jobId, kind, stage: "", message: "", pct: -1, stagePct: -1, files: [], ...more });
  const samples = {
    find: (j) => status(j, "making", { stage: "find", pct: 0.04 }),
    download: (j, f) => status(j, "making", { stage: "download", pct: 0.1 + f * 0.5, stagePct: f }),
    gif: (j) => status(j, "making", { stage: "gif", pct: 0.7 }),
    mp4: (j, f) => status(j, "making", { stage: "mp4", pct: 0.75 + f * 0.24, stagePct: f }),
    live: (j) => status(j, "making", { stage: "download", pct: -1, stagePct: -1 }),
    done: (j) => status(j, "done", { pct: 1, files: [file("gif", 2516582), file("mp4", 1887436)] }),
    already: (j) => status(j, "already", { pct: 1, files: [file("gif", 2516582)] }),
    partial: (j) => status(j, "partial", { files: [file("gif", 2516582)], message: "The MP4 couldn't be made." }),
    failed: (j) => status(j, "failed", { message: "Couldn't get this video. It may be private or removed." }),
    ready: (j) => status(j, "ready"),
  };

  let timers = [];
  const schedule = (ms, fn) => timers.push(setTimeout(fn, ms));

  // HH:MM:SS, MM:SS or SS with decimals on the last part, like the CLI.
  const parse = (t) => {
    const parts = t.trim().split(":");
    if (parts.length > 3 || parts.some((p) => !/^\d+(\.\d+)?$/.test(p))) return NaN;
    return parts.reduce((acc, p) => acc * 60 + Number(p), 0);
  };

  const App = {
    async Settings() { return structuredClone(settings); },
    async Boot() {
      return {
        needsSetup: params.has("setup"), needsAdmin: false,
        settings: structuredClone(settings), themes: ["grey", "mint", "lilac"], binDir,
      };
    },
    async Install() {
      const total = 250e6;
      for (let i = 1; i <= 20; i++) later(i * 150, () => emit("setup", { tool: "ffmpeg", done: (total * i) / 20, total }));
      if (params.get("setup") === "fail") {
        later(1600, () => emit("setup:error", { message: "Couldn't download the helper files. Check your connection and try again.", needsAdmin: false }));
      } else {
        later(3200, () => emit("setup:done", null));
      }
    },
    async Add(text) {
      // like lookup.ParseInput: a link pasted without its scheme gets https:// back
      const links = text.split(/[\s,]+/)
        .map((l) => (/^[A-Za-z0-9.-]+\.[A-Za-z][A-Za-z0-9-]*(:\d+)?([/?#]|$)/.test(l) ? "https://" + l : l))
        .filter((l, i, all) => /^https?:\/\/./i.test(l) && all.indexOf(l) === i);
      const placeholders = links.map((url) => ({ ...slip(url, { title: "", durationSec: 0 }) }));
      placeholders.forEach((p) => later(700, () => {
        if (/private|removed/.test(p.url)) {
          emit("slips:replace", { id: p.id, source: "", slips: [{ ...slip(p.url, { title: "", durationSec: 0 }), failed: true, message: "Couldn’t get this video. It may be private or removed." }] });
        } else if (/[?&]list=/.test(p.url) && !/[?&]v=/.test(p.url)) {
          emit("slips:replace", { id: p.id, source: "Family videos 2025", slips: playlist.map((v) => slip(v.unavailable ? "" : p.url + "#" + nextId, v)) });
        } else {
          emit("slips:replace", { id: p.id, source: "Pasted links", slips: [slip(p.url, playlist[1])] });
        }
      }));
      return placeholders;
    },
    async Validate(start, end, durationSec) {
      const bad = "That time doesn't look right. Try 1:23 or 0:01:23.";
      if (!start.trim()) return { start: 0, end: 0, message: "Add a start time to include this one." };
      const s = parse(start);
      if (Number.isNaN(s)) return { start: 0, end: 0, message: bad };
      let e = 0;
      if (end.trim()) {
        e = parse(end);
        if (Number.isNaN(e)) return { start: 0, end: 0, message: bad };
        if (e <= s) return { start: 0, end: 0, message: "Stop must be after Start." };
      }
      if (durationSec > 0 && s >= durationSec) return { start: 0, end: 0, message: "That's past the end of the video." };
      if (durationSec > 0 && e > durationSec) return { start: 0, end: 0, message: "Stop is past the end of the video." };
      return { start: s, end: e, message: "" };
    },
    // Each job walks find -> download -> gif -> mp4 -> an end state; the
    // end states cycle through done, partial, failed and already.
    async Make(jobs) {
      const ends = [samples.done, samples.partial, samples.failed, samples.already];
      let t = 0;
      jobs.forEach((j, n) => {
        schedule(t, () => emit("slip:status", samples.find(j.id)));
        for (let i = 1; i <= 10; i++) schedule(t + 400 + i * 120, () => emit("slip:status", samples.download(j.id, i / 10)));
        schedule(t + 1800, () => emit("slip:status", samples.gif(j.id)));
        for (let i = 1; i <= 5; i++) schedule(t + 2400 + i * 120, () => emit("slip:status", samples.mp4(j.id, i / 5)));
        schedule(t + 3200, () => emit("slip:status", ends[n % ends.length](j.id)));
        t += 3400;
      });
      schedule(t, () => { timers = []; emit("run:finished", { stopped: false }); });
    },
    async Stop() {
      timers.forEach(clearTimeout);
      timers = [];
      emit("run:finished", { stopped: true });
    },
    async SaveSettings(s) {
      if (!s.formats?.length) throw "Pick at least one format.";
      settings = structuredClone(s);
      return null;
    },
    async PickFolder() { return "D:\\Clips from the shop"; },
    async OpenFolder(path) { console.info("[dev] OpenFolder", path); },
    async OpenLogs() { console.info("[dev] OpenLogs"); },
  };

  window.go = { main: { App } };
  window.runtime = {
    EventsOn(name, cb) {
      (listeners[name] ||= []).push(cb);
      return () => { listeners[name] = listeners[name].filter((f) => f !== cb); };
    },
    EventsOff(...names) { names.forEach((n) => delete listeners[n]); },
    EventsEmit: emit,
  };
  window.devSamples = samples; // for poking states from the console
})();
