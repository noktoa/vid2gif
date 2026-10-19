import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { ratio, parseThemes } from "./contrast.js";

const INK = "#1E1B24", INK2 = "#4A4655", GREEN = "#00805A", ALERT = "#C23B22", YELLOW = "#FFD60A";

test("ratio matches known WCAG values", () => {
  assert.equal(ratio("#000000", "#FFFFFF").toFixed(1), "21.0");
  assert.equal(ratio("#777777", "#FFFFFF").toFixed(2), "4.48");
});

test("themes.css defines grey, mint and lilac", () => {
  const themes = parseThemes(readFileSync(new URL("./themes.css", import.meta.url), "utf8"));
  assert.deepEqual(Object.keys(themes).sort(), ["grey", "lilac", "mint"]);
  for (const [name, t] of Object.entries(themes)) {
    for (const key of ["ground", "slip", "line", "tb"]) assert.match(t[key] ?? "", /^#/, `${name} --${key}`);
  }
});

test("text keeps 4.5:1 in every theme", () => {
  const themes = parseThemes(readFileSync(new URL("./themes.css", import.meta.url), "utf8"));
  for (const [name, t] of Object.entries(themes)) {
    const pairs = { "ink on ground": [INK, t.ground], "ink on titlebar": [INK, t.tb], "ink on slip": [INK, t.slip],
      "secondary on slip": [INK2, t.slip], "secondary on ground": [INK2, t.ground],
      "green on slip": [GREEN, t.slip], "error on slip": [ALERT, t.slip] };
    for (const [what, [fg, bg]] of Object.entries(pairs)) {
      assert.ok(ratio(fg, bg) >= 4.5, `${name}: ${what} is ${ratio(fg, bg).toFixed(2)}`);
    }
  }
});

test("ink on the yellow button is at least 7:1", () => {
  assert.ok(ratio(INK, YELLOW) >= 7, ratio(INK, YELLOW).toFixed(2));
});

test("focus rings keep 3:1: ink on white, yellow on a pressed (ink) button", () => {
  assert.ok(ratio(INK, "#FFFFFF") >= 3);
  assert.ok(ratio(YELLOW, INK) >= 3);
});
