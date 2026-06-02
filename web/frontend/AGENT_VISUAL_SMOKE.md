# Agent Glass UI Visual Smoke Checklist

This checklist defines the lightweight browser and responsive acceptance pass for the `/agent` cockpit glass UI. It is intentionally manual/documentation-first: the frontend currently has Vitest coverage but no repository-standard Playwright, Cypress, axe, Storybook, or visual-regression harness.

## Scope

- Target route: `/agent`
- Primary runtime: `http://127.0.0.1:8080/agent`
- Optional reference runtime: `http://127.0.0.1:5175/agent` when an existing process is already available
- Do not kill, restart, or replace existing local processes for this check.
- Do not use this checklist to change SSE reconnect logic, reducer state shape, API payloads, route splitting, or mobile navigation behavior.

## Viewports

Run the smoke pass at these viewport sizes:

| Viewport | Size | Purpose |
| --- | --- | --- |
| Desktop | `1440x900` | Primary cockpit layout with TaskTree, center timeline, and ToolInspector visible. |
| Tablet | `1024x768` | Medium-density layout and wrapping behavior. |
| Mobile | `390x844` | Narrow viewport readability; verify no page-level horizontal scroll. |

## Optional local browser commands

The repository does not require Playwright. If the Codex Playwright CLI wrapper is available locally, these commands can be used for manual screenshots without adding project dependencies:

```bash
export PWCLI="$HOME/.codex/skills/playwright/scripts/playwright_cli.sh"
mkdir -p output/playwright
"$PWCLI" open http://127.0.0.1:8080/agent
"$PWCLI" snapshot
"$PWCLI" screenshot output/playwright/agent-desktop-1440x900.png
```

For tablet/mobile screenshots, set the viewport using the local browser tooling available in the environment, then write screenshots under `output/playwright/`. Do not commit generated screenshots unless a separate review explicitly requests them.

## Glass UI checks

- Background gradient and glass surfaces are visible but do not reduce text readability.
- Main cockpit shell, TaskTree, TurnTimeline, Composer, ToolInspector, and banners use a consistent depth hierarchy.
- Nested glass does not feel overly heavy; ToolInspector should not appear as a thick double-card inside the right rail.
- Borders/highlights/shadows are visible enough to separate regions, but not so strong that hover/focus states dominate the view.
- Selected timeline tool card is distinguishable from hover-only cards.

## Browser fallback checks

- In browsers without `backdrop-filter`, glass panels fall back to a solid readable background.
- Text, icons, status pills, and code blocks remain readable against fallback surfaces.
- No control depends on blur alone to communicate hierarchy or state.

## Reduced motion checks

With `prefers-reduced-motion: reduce` enabled:

- Hover and focus states remain understandable.
- Decorative motion is reduced or disabled.
- Interactive cards do not jump or translate in a way that can distract from reading.
- Loading spinners remain acceptable for progress indication.

## Accessibility checks

- Keyboard Tab order is usable through TaskTree, cockpit toolbar, timeline tool cards, Composer, and ToolInspector.
- Focus rings are visible on icon-only buttons, quick prompt chips, timeline tool cards, and Composer actions.
- Icon-only buttons have accessible names, especially refresh, trace, download, clear, close, pause, cancel, and delete controls.
- Banners and status pills do not rely on color alone; icon/text labels convey meaning.
- Normal body text and compact metadata remain readable at the checked viewport sizes.

## ToolInspector and output readability

- ToolInspector header is not duplicated or visually competing with the right rail header.
- Call summary, command result, error, metadata, and result sections remain visually separated.
- Long `metadata`, `result`, `stdout`, and `stderr` content remains readable and does not overflow the page.
- Code/output blocks use a sufficiently solid background; avoid overly transparent JSON/log surfaces.
- Long single-line values wrap or scroll within the panel without creating page-level horizontal scroll.

## Timeline and Composer checks

- Turn groups remain readable with multiple messages, tool cards, approval events, and memory compaction alerts.
- Tool card hover state is subtle; selected/focused state is still clear.
- Composer quick prompts remain accessible and horizontally scroll only inside their rail when needed.
- Composer textarea remains usable at desktop, tablet, and mobile widths.
- Sending, continue, pause, cancel, and new-task controls remain visible or intentionally compact according to the current responsive design.

## Banner checks

Validate these banner types when present:

- Error banner
- Run summary banner
- Authorization/approval banner
- Timeline approval event
- Memory compaction alert

Acceptance criteria:

- Text contrast is sufficient.
- Icon and label communicate the state.
- Long text truncates or wraps without breaking layout.
- Banner spacing does not push Composer or timeline controls out of view unexpectedly.

## Responsive acceptance

- Desktop: TaskTree, central cockpit, and ToolInspector align without overlap.
- Tablet: content remains readable; wrapping does not hide critical controls.
- Mobile: no page-level horizontal scroll; secondary panels follow the existing responsive behavior.
- Do not treat mobile navigation redesign as part of this smoke pass.

## Pass/fail recording

Record findings in the implementation or QA note with:

- Runtime URL used
- Viewport size
- Screenshot path if captured
- Pass/fail result
- Short issue summary
- Evidence path or screenshot path
- Whether the issue blocks release or is follow-up polish
