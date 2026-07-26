# Interface Design System — panya-charge-oss

## Direction

**Graphite & Amber** — operating well-engineered hardware. Calm, dense, trustworthy. The palette is rooted in the charger's physical world: brushed-aluminium graphite for structure, amber (the charging LED color) as the single accent.

## Tokens

```css
--canvas:#1a1b1e;          /* base background — brushed aluminium */
--surface:#212228;          /* +1 elevation — fieldsets, cards */
--surface-2:#282a31;        /* +2 elevation — inset inputs */
--surface-3:#2f3239;        /* +3 elevation — active/hover */
--border:rgba(255,255,255,0.07);        /* disappears until needed */
--border-strong:rgba(255,255,255,0.12); /* focus-adjacent */
--ink:#e7e8ec;              /* primary text */
--ink-2:#a4a7b0;            /* secondary — labels */
--ink-3:#6a6d76;            /* muted — helpers, metadata */
--accent:#e0a449;           /* amber — Save, focus, active ONLY */
--accent-hover:#edb251;
--accent-dim:rgba(224,164,73,0.15);     /* focus glow */
--success:#7fa86a;          /* muted sage — hot-apply badge */
--success-dim:rgba(127,168,106,0.15);
--warn:#c89446;             /* amber family — rebuild badge */
--warn-dim:rgba(200,148,70,0.15);
--danger:#b5624e;           /* muted clay — errors */
--danger-dim:rgba(181,98,78,0.15);
```

## Depth Strategy

**Surface elevation only.** No shadows, no decorative borders. Hierarchy emerges from whisper-quiet lightness shifts between canvas → surface → surface-2 → surface-3. Each step is a few percentage points — barely visible in isolation, felt when stacked.

Borders use `rgba(255,255,255,0.07)` — low-opacity, blends with background. They define edges without demanding attention.

## Typography

- **Family:** `system-ui, -apple-system, sans-serif` — no external fonts
- **Numeric inputs:** `font-variant-numeric: tabular-nums` — amp/second/port values align
- **Title:** `1rem`, `font-weight:600`, tight letter-spacing
- **Section names:** `.7rem`, uppercase, letter-spaced — quiet structural markers
- **Labels:** `.8rem`, ink-2
- **Helper text:** `.7rem`, ink-3

## Spacing

Base unit **4px**. Scale: 4, 8, 12, 16, 20, 24, 32.

## Component Patterns

### Sections
Fieldset replacement. `background:var(--surface)`, `border-radius:10px`. Section name is uppercase letter-spaced ink-3.

### Field Rows
Flex layout: label (150px basis) / input (flex:1) / badges. Row separator: `border-top:1px solid var(--border)`.

### Inputs
Inset — `background:var(--surface-2)` (darker than fieldset). Border transparent. Focus: `border-color:var(--accent)` + `box-shadow:0 0 0 3px var(--accent-dim)`. Disabled: `opacity:.5`.

### Badges
Four variants, each with dim background + semantic text color:
- `.b-env` — neutral, "ENV" (field overridden by environment variable)
- `.b-hot` — sage, "instant" (applies without charger disconnect)
- `.b-rebuild` — amber-warn, "disconnect" (brief charger disconnect)
- `.b-restart` — neutral, "restart" (applies on next process restart)

### Result Fragments
`.frag` base + modifier. Left-border accent (3px solid semantic color), dim background, semantic text. Calm, not loud.

### Save Bar
`position:sticky;bottom:0`. Gradient fade from transparent to canvas. Right-aligned. Primary button: amber bg, dark text, `font-weight:600`.

## Signature

**Consequence-aware form.** Every field shows what will happen when you change it — instant, disconnect, or restart — before you commit. This is the product's core value: configure with confidence. The badge system elevates this, grouping fields by physical consequence.

## Files

- `internal/adapter/inbound/webui/static/app.css` — all tokens + component styles
- `internal/adapter/inbound/webui/templates/*.html` — templates linking app.css
