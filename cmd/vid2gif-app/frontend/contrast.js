// WCAG contrast helpers for the theme check in contrast.test.js.

function luminance(hex) {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function ratio(a, b) {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// parseThemes reads every [data-theme="name"] block of themes.css into
// { name: { ground, slip, line, tb } }.
export function parseThemes(css) {
  const out = {};
  for (const [, name, body] of css.matchAll(/\[data-theme="(\w+)"\][^{]*\{([^}]*)\}/g)) {
    out[name] = {};
    for (const [, key, val] of body.matchAll(/--(\w+)\s*:\s*(#[0-9a-fA-F]{6})/g)) out[name][key] = val;
  }
  return out;
}
