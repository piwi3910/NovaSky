# NovaSky UI Design Review — Full Comprehensive

**Reviewed:** 2026-04-16
**Target:** Entire web/src/ (17 pages, 2 hooks, 1 component)
**Focus:** Visual, Usability, Code Quality, Performance, Responsive
**Platform:** Desktop, tablet, mobile

## Summary

The UI is functional but has systematic gaps: no error/loading states across the entire app, multiple WebSocket connections leaking, hardcoded colors violating the new design system, and no mobile touch support for interactive canvas elements. The settings pages are well-standardized but all silently swallow save failures.

**Issues Found: 51**
- Critical: 5
- Major: 16
- Minor: 19
- Suggestions: 11

---

## Consolidated Findings

| # | Sev | Cat | Issue | Location |
|---|-----|-----|-------|----------|
| 1 | Crit | Perf | Multiple WebSocket connections per tab — each useWebSocket() opens its own | useWebSocket.ts, App.tsx:25, Dashboard.tsx:19 |
| 2 | Crit | Perf | WebSocket reconnect loop never stops on unmount — leaks timers | useWebSocket.ts:18 |
| 3 | Crit | UX | useApi hook silently swallows all errors — no error state exposed | useApi.ts:15 |
| 4 | Crit | UX | No loading indicators on any data-dependent page | Dashboard, Frames, History, FocusMode |
| 5 | Crit | Code | No ErrorBoundary anywhere — single crash kills entire app | App.tsx |
| 6 | Maj | Perf | useApi fetches empty URL when frameId undefined — silent HTML parse failure | Dashboard.tsx:27 |
| 7 | Maj | UX | No error handling on save operations — saving stays true forever on failure | All 13 settings/editor pages |
| 8 | Maj | Visual | Dashboard charts use wrong colors — violates design system | Dashboard.tsx:212,225,233-234 |
| 9 | Maj | Visual | OverlayEditor cards missing shadow="sm" padding="lg", wrong Title order | OverlayEditor.tsx:153,156,170,218 |
| 10 | Maj | UX | Frames page — no empty state message | Frames.tsx:17-28 |
| 11 | Maj | UX | History page — no empty state, no pagination | History.tsx:13-27 |
| 12 | Maj | UX | FocusMode SimpleGrid cols={4} not responsive — breaks on mobile | FocusMode.tsx:53 |
| 13 | Maj | UX | ProcessingTuner fake loading state — never actually visible | ProcessingTuner.tsx:18-25 |
| 14 | Maj | UX | SettingsDisk crashes if disk data is null (NaN in Progress bar) | SettingsDisk.tsx:33,42-45 |
| 15 | Maj | Perf | Canvas overlay doesn't redraw on window resize — misalignment | Dashboard.tsx |
| 16 | Maj | Perf | FocusMode image cache-busting on every render — unnecessary re-downloads | FocusMode.tsx:64 |
| 17 | Maj | Code | FrameMasking draw dependency stale — drag uses old values after new frame | FrameMasking.tsx:109 |
| 18 | Maj | Code | Pervasive `any` types — 35+ occurrences, no API response interfaces | All pages |
| 19 | Maj | UX | OverlayEditor bypasses useApi, silent error swallowing | OverlayEditor.tsx:56-69 |
| 20 | Maj | UX | SettingsStorage — no SMB config fields shown | SettingsStorage.tsx:55-65 |
| 21 | Maj | UX | FocusMode exposure/gain inputs are dead controls — never sent to API | FocusMode.tsx:23-28 |
| 22 | Min | Visual | Dashboard astronomy card inconsistent typography | Dashboard.tsx:245-264 |
| 23 | Min | Visual | Dashboard status cards wrong data typography pattern | Dashboard.tsx:271-301 |
| 24 | Min | Visual | Missing monospace for ADU/exposure values | Dashboard.tsx:151, Frames.tsx:22 |
| 25 | Min | Visual | Dashboard chart tooltip missing dark background | Dashboard.tsx:209-211 |
| 26 | Min | Visual | Chart axis text missing design system color | Dashboard.tsx:209,227-228 |
| 27 | Min | Visual | OverlayEditor hardcoded dark-5 background | OverlayEditor.tsx:195 |
| 28 | Min | Code | FocusMode star data computed on every render — needs useMemo | FocusMode.tsx:18-21 |
| 29 | Min | Code | Dashboard component too large (305 lines) | Dashboard.tsx |
| 30 | Min | Code | PipelineView magic offset marginLeft: 160 | PipelineView.tsx:134 |
| 31 | Min | Resp | FrameMasking only binds mouse events, not touch | FrameMasking.tsx:186-189 |
| 32 | Min | UX | Fullscreen overlay no Escape key handler | Dashboard.tsx:174 |
| 33 | Min | Code | ProcessingTuner async keyword misleading — no actual await | ProcessingTuner.tsx:17 |
| 34 | Min | Perf | No route-level code splitting — all 18 pages eagerly loaded | App.tsx |
| 35 | Min | Code | Settings pages duplicate config-load pattern — extract useConfig hook | All settings pages |
| 36 | Min | Code | useApi doesn't support conditional fetching | useApi.ts |
| 37 | Min | Visual | SettingsCamera multi-declarations on single lines | SettingsCamera.tsx:7-8 |
| 38 | Min | UX | No active state on NavLink items | App.tsx:40-59 |
| 39 | Min | UX | No confirmation dialog on OverlayEditor delete | OverlayEditor.tsx:148 |
| 40 | Min | UX | No success toast after save operations | All settings pages |
| 41 | Sug | Code | Adopt TanStack Query / SWR for data fetching | useApi.ts replacement |
| 42 | Sug | Resp | AppShell navbar no mobile toggle button | App.tsx:30 |
| 43 | Sug | Perf | Lazy-load Recharts section | Dashboard.tsx |
| 44 | Sug | Code | Use Mantine useDisclosure for boolean toggles | Dashboard.tsx |
| 45 | Sug | Code | scalePoint callback has empty deps — should be plain function | Dashboard.tsx:46-49 |
| 46 | Sug | UX | FrameMasking no message when no frame available | FrameMasking.tsx |
| 47 | Sug | UX | ProcessingTuner saveToProfile no loading/success feedback | ProcessingTuner.tsx:29-33 |
| 48 | Sug | Visual | Consider Mantine notification system for save feedback | Global |
| 49 | Sug | Code | Define TypeScript interfaces for all API responses | Global |
| 50 | Sug | Resp | Mobile nav hamburger menu needed | App.tsx |
| 51 | Sug | UX | Add ResizeObserver for canvas overlay alignment | Dashboard.tsx |

---

## Positive Observations

- Settings pages follow a well-standardized pattern (useApi, useRef initialized, save function)
- Mantine components used consistently (Card, Stack, Group, Badge, Switch, NumberInput)
- WebSocket provides real-time auto-exposure data — good for live monitoring
- Frame masking has intuitive drag interaction
- Calibration log window is excellent UX for long-running operations
- Navigation organized logically with Settings as collapsible group

## Recommended Fix Priority

**Phase 1 — Foundation (fixes most issues at once):**
1. Fix useApi hook — add error state, loading consumption, conditional fetching (#3, #4, #6, #36)
2. Lift WebSocket into context provider with proper cleanup (#1, #2)
3. Add ErrorBoundary wrapper (#5)
4. Wrap all save functions in try/catch/finally with notifications (#7, #40)

**Phase 2 — Design System Compliance:**
5. Update chart colors to design system tokens (#8, #25, #26)
6. Fix card patterns across all pages (#9)
7. Add empty states to Frames, History (#10, #11)
8. Fix responsive breakpoints (#12, #31)

**Phase 3 — Polish:**
9. Fix SettingsDisk null crash (#14)
10. Remove dead controls (#21), add missing SMB fields (#20)
11. Add Escape key handler, touch events (#32, #31)
12. Type safety — define API response interfaces (#18, #49)
