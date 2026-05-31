# Product

## Register

product

## Users

Administrators operating a Hysteria 2 proxy panel. They sit at a desk, monitor node health, manage users, and diagnose connection issues. The interface is used for brief, focused operational checks throughout the day.

## Product Purpose

A lightweight admin panel for Hysteria 2 nodes. Centralizes node monitoring, user management, traffic aggregation, and real-time diagnostics. Success is when an admin can assess fleet health and troubleshoot a user issue in under 60 seconds.

## Brand Personality

Dense, technical, calm. Instrument-grade, not decorative. The north-star is **Linear**: list-first, hairline-defined, low-contrast, precise.

## Anti-references

Marketing landing pages, SaaS onboarding funnels, playful consumer apps. No hero sections, no illustration, no decorative motion.

## Design Principles

1. Information density over whitespace. Show more, scroll less.
2. State over decoration. Color and motion convey node health and user status, not brand flair.
3. Instrument confidence. Every label, value, and status indicator must read as trustworthy and precise.
4. Consistent vocabulary. Same component shapes and patterns across every surface.

## Design Language

The concrete expression of the principles above. Tokens live in `frontend/src/styles/globals.css` (overriding HeroUI v3 defaults); changes to this language should land there.

- **Theme.** System-aware light and dark, following the OS (`prefers-color-scheme`); no manual toggle. No-flash on SSR via an inline `<head>` script in `frontend/src/routes/__root.tsx` that sets the theme class before first paint, plus a live listener for mid-session switches.
- **Color (Restrained).** Tinted-neutral surfaces, never pure black or white. Low contrast: panels are defined by hairline borders, not elevation. One accent, indigo `#5E6AD2` (`oklch(0.54 0.16 274)`, lifted to `oklch(0.62 0.15 274)` in dark), used only for links, focus, primary actions, and selection. Green / amber / red are reserved strictly for status (health, active vs disabled).
- **Typography.** Inter, self-hosted via `@fontsource-variable/inter`. Tight tracking (`-0.011em`, tighter on headings), ~13px app base, ~1.125 to 1.2 step ratio. Monospace with `tabular-nums` for all data: bytes, latency, intervals, auth keys, endpoints, timestamps.
- **Surfaces and layout.** Hairline borders throughout, `--radius: 0.5rem`, subtle row hover. Prefer dense tables and lists over cards. Summaries are one connected rail, not free-floating metric tiles.
- **State indicators.** Small colored status dots, not badge pills. Disabled rows dim. Errors truncate inline with a full-text tooltip.
- **Motion.** 150 to 250 ms on hover and state transitions only. No page-load choreography, no decorative motion.
- **Interaction patterns.** Skeleton rows on load, never a centered spinner. Empty states teach the next action (no illustration). A live instrument feel: silent background refresh with a relative "updated Ns ago" readout. Low-friction affordances like click-to-copy where they save a step.

## Avoid

The AI-dashboard tells, in addition to the anti-references above: hero-metric card rows (big number, label, accent), identical card grids, nested cards, gradient text, decorative glassmorphism, colored side-stripe borders, and em dashes in UI copy.

## Accessibility & Inclusion

Standard defaults. Semantic HTML, visible focus states, keyboard navigation. No special WCAG tier targeted beyond browser defaults.
