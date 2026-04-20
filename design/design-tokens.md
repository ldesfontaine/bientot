# Bientôt — Design tokens

Dark-first, cool-neutral base, teal accent, semantic green/amber/red/gray.
Every token maps to a CSS variable in the reference prototypes and is ready to
drop into `tailwind.config.js` (`theme.extend`).

---

## 1. Colors

### Backgrounds (cool-neutral dark)

| Token              | Hex       | Role                               |
|--------------------|-----------|------------------------------------|
| `--bg-0`           | `#0b0d10` | Page background                    |
| `--bg-1`           | `#101317` | Elevated surface (cards, sidebar)  |
| `--bg-2`           | `#161a20` | Hover / nested surface             |
| `--bg-3`           | `#1d2229` | Pressed / active row               |

### Borders

| Token               | Hex       | Role                               |
|---------------------|-----------|------------------------------------|
| `--border-subtle`   | `#1c2028` | Card dividers, table rows          |
| `--border`          | `#232832` | Default border                     |
| `--border-strong`   | `#2e3440` | Hover, focus edge                  |

### Text

| Token       | Hex       | Role                                      |
|-------------|-----------|-------------------------------------------|
| `--text-0`  | `#e6e8eb` | Primary text                              |
| `--text-1`  | `#a8adb6` | Secondary text                            |
| `--text-2`  | `#6b727d` | Muted / labels / captions                 |
| `--text-3`  | `#4a5059` | Disabled / placeholder                    |

### Accent — teal (brand, info, interactive focus)

| Token              | Value                         | Role                          |
|--------------------|-------------------------------|-------------------------------|
| `--accent`         | `oklch(72% 0.09 195)`         | Accent base (stroke, active)  |
| `--accent-dim`     | `oklch(60% 0.08 195)`         | Gradient bottom, pressed      |
| `--accent-bg`      | `oklch(72% 0.09 195 / 0.12)`  | Tinted background             |
| `--accent-border`  | `oklch(72% 0.09 195 / 0.28)`  | Tinted border                 |

### Semantic

| Semantic | Base                           | Background                          | Border                             |
|----------|--------------------------------|-------------------------------------|------------------------------------|
| ok       | `oklch(72% 0.15 150)` (green)  | `oklch(72% 0.15 150 / 0.12)`        | `oklch(72% 0.15 150 / 0.3)`        |
| warn     | `oklch(80% 0.14 80)`  (amber)  | `oklch(80% 0.14 80 / 0.12)`         | `oklch(80% 0.14 80 / 0.3)`         |
| crit     | `oklch(66% 0.19 25)`  (red)    | `oklch(66% 0.19 25 / 0.14)`         | `oklch(66% 0.19 25 / 0.32)`        |
| offline  | `#4a5059` (`--muted-dot`)      | —                                   | —                                  |

Semantic colors are reserved for status. The teal accent is **never** mixed
into an ok/warn/crit role.

---

## 2. Typography

### Fonts

- **Sans (UI):** `Geist`, ui-sans-serif, system-ui, -apple-system, sans-serif
- **Mono (numerics, IDs, logs, timestamps):** `Geist Mono`, ui-monospace, "SF Mono", Menlo, monospace
- Geist OpenType features enabled: `cv11`, `ss01`, `ss03`
- Geist Mono OpenType features enabled on `.mono`: `zero`, `ss01`

### Scale

| Token        | Size  | Typical use                                         |
|--------------|-------|-----------------------------------------------------|
| `--fs-xs`    | 11 px | Caption / uppercase label                           |
| `--fs-sm`    | 12 px | Meta, hint, mono meta row                           |
| `--fs-base`  | 13 px | Body default                                        |
| `--fs-md`    | 14 px | Subsection title, brand, form label                 |
| `--fs-lg`    | 16 px | Module card metric                                  |
| `--fs-xl`    | 20 px | Section title, chart value                          |
| `--fs-2xl`   | 24 px | KPI value                                           |
| `--fs-3xl`   | 32 px | Page title                                          |

### Weights

- 400 — body, mono numerics
- 500 — section titles, subsection, KPI value, primary button, brand
- 600 — page title, brand mark
- 700 — brand mark only

### Line heights

- `--lh-tight`: 1.2 — KPI values, large numerics
- `--lh-normal`: 1.45 — body default

### Letter-spacing

- Page title: `-0.02em`
- Section title: `-0.01em`
- Subsection / small titles: `-0.005em`
- KPI value: `-0.02em`
- Uppercase labels: `+0.06em`

### Typical patterns

| Pattern           | Recipe                                                              |
|-------------------|---------------------------------------------------------------------|
| Page title        | 20 px · 600 · `-0.01em`                                             |
| KPI value         | mono · 24 px · 500 · `-0.02em` · line-height 1.1                    |
| Chart value       | mono · 20 px · 500 · `-0.02em`                                      |
| Module metric     | mono · 16 px · 500 · `-0.01em`                                      |
| Uppercase label   | 11 px · text-2 · uppercase · `+0.06em`                              |
| Log line          | mono · 12 px                                                        |

---

## 3. Spacing (4 px base)

| Token     | Value |
|-----------|-------|
| `--s-1`   | 4 px  |
| `--s-2`   | 8 px  |
| `--s-3`   | 12 px |
| `--s-4`   | 16 px |
| `--s-5`   | 20 px |
| `--s-6`   | 24 px |
| `--s-8`   | 32 px |
| `--s-10`  | 40 px |
| `--s-12`  | 48 px |

Typical:

- Card padding: `--s-4` (16 px) comfortable / 12 px compact
- Content padding: `--s-6` vertical, `--s-8` horizontal
- Grid gap: `--s-3` (12 px)
- Section spacing: `--s-8` (32 px)

---

## 4. Border radius

| Token     | Value | Use                                |
|-----------|-------|------------------------------------|
| `--r-sm`  | 4 px  | Inputs, tags, skeletons            |
| `--r-md`  | 6 px  | Buttons, selects, banners, pills   |
| `--r-lg`  | 8 px  | Cards, tables, logs, drawer body   |
| `--r-xl`  | 10 px | Container surfaces (charts frame)  |

Design is flat by default — radii are restrained.

---

## 5. Elevation / shadows

Flat by default. Shadows appear only on overlays.

| Token              | Value                                  | Use       |
|--------------------|----------------------------------------|-----------|
| `--shadow-drawer`  | `-24px 0 48px -12px rgba(0,0,0,0.5)`   | Drawer    |
| `--shadow-popover` | `0 8px 24px -8px rgba(0,0,0,0.6)`      | Tooltip, menu, tweaks panel |

No glows, no gradients beyond the brand mark and the tinted KPI variants.

---

## 6. Density

Two modes via `data-density` on `<html>`:

| Token         | Comfortable (default) | Compact |
|---------------|-----------------------|---------|
| `--row-h`     | 40 px                 | 32 px   |
| `--card-pad`  | 16 px                 | 12 px   |
| `--fs-base`   | 13 px                 | 12 px   |
| `--fs-md`     | 14 px                 | 13 px   |

Layout constants (shared across densities):

- Sidebar width: `--sidebar-w` = 232 px
- Topbar height: 48 px
- Drawer width: `min(720px, 92vw)`
- Max content width: 1400 px

---

## 7. Motion

- Default transitions: `120 ms` linear/ease for color and border changes
- Drawer slide: `220 ms cubic-bezier(0.22, 1, 0.36, 1)`
- Pulse dot / live indicator: `1.8 s` ease-in-out infinite, opacity 1 ↔ 0.45
- Skeleton shimmer: `1.4 s` linear infinite
- Top-bar auto-refresh pulse: `2 s` ease-in-out infinite, opacity 0.4 ↔ 1

---

## 8. Charts

- Single thin stroke (1.5 px) per series, `vector-effect: non-scaling-stroke`
- 1 px gridlines in `--border-subtle`, 4 horizontal ticks, 3 x-axis ticks
- Axis labels: Geist Mono 10 px `--text-2`
- Multi-series legend colors:
  - primary → `--accent`
  - secondary → `oklch(72% 0.09 220)` (slightly bluer teal)
  - tertiary → `--text-2`
- Data gaps: 45°-rotated hatch pattern with centered "NO DATA" mono caption

---

## 9. Tailwind mapping (drop-in for `theme.extend`)

```js
colors: {
  bg:     { DEFAULT: "#0b0d10", elevated: "#101317", hover: "#161a20", active: "#1d2229" },
  border: { subtle: "#1c2028", DEFAULT: "#232832", strong: "#2e3440" },
  fg:     { DEFAULT: "#e6e8eb", muted: "#a8adb6", subtle: "#6b727d", disabled: "#4a5059" },
  accent: { DEFAULT: "oklch(72% 0.09 195)", dim: "oklch(60% 0.08 195)" },
  ok:     { DEFAULT: "oklch(72% 0.15 150)" },
  warn:   { DEFAULT: "oklch(80% 0.14 80)"  },
  crit:   { DEFAULT: "oklch(66% 0.19 25)"  },
},
fontFamily: {
  sans: ['Geist', 'ui-sans-serif', 'system-ui'],
  mono: ['"Geist Mono"', 'ui-monospace', '"SF Mono"', 'Menlo'],
},
fontSize: {
  xs:   ['11px', { lineHeight: '1.45' }],
  sm:   ['12px', { lineHeight: '1.45' }],
  base: ['13px', { lineHeight: '1.45' }],
  md:   ['14px', { lineHeight: '1.45' }],
  lg:   ['16px', { lineHeight: '1.45' }],
  xl:   ['20px', { lineHeight: '1.2'  }],
  '2xl':['24px', { lineHeight: '1.1'  }],
  '3xl':['32px', { lineHeight: '1.2'  }],
},
spacing: {
  1: '4px', 2: '8px', 3: '12px', 4: '16px', 5: '20px',
  6: '24px', 8: '32px', 10: '40px', 12: '48px',
},
borderRadius: {
  sm: '4px', md: '6px', lg: '8px', xl: '10px',
},
boxShadow: {
  drawer:  '-24px 0 48px -12px rgba(0,0,0,0.5)',
  popover: '0 8px 24px -8px rgba(0,0,0,0.6)',
},
```
