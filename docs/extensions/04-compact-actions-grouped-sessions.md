---
title: "Extension: Compact Actions and Grouped Sessions UI"
---

# Extension: Compact Actions and Grouped Sessions UI

## Problem Statement

The workspace portal currently uses text-heavy action controls and a flat running-sessions list that consumes too much horizontal space and vertical attention, especially on mobile devices.

From a user perspective, this creates three daily friction points:

1. Directory actions are visually noisy because each row repeats three text buttons.
2. Running sessions are hard to scan because each row shows full absolute paths.
3. Session management is cognitively expensive because running sessions are not grouped by directory, forcing users to manually correlate sessions with the directory list above.

This reduces at-a-glance clarity in the exact UI where users make frequent launch/open/stop decisions.

## Solution

Redesign the UI to make actions and sessions compact, grouped, and mobile-resilient while preserving the current session lifecycle behavior.

From the user perspective, the new experience is:

- Directory actions use icon-only buttons (official OpenCode and VS Code icons, plus a style-matching docs icon) with tooltips and accessible labels.
- Running sessions are displayed above the directory tree and grouped by directory.
- Each directory group shows running session types as icon buttons that open the same URLs as the current Open action.
- Long absolute paths are replaced with workspace-relative labels, with a root-directory exception that shows only the root directory name.
- Stopping sessions moves from one stop button per row to one per-directory stop dropdown where users pick exactly which running session type to stop.

## User Stories

1. As a portal user, I want directory action buttons to be icon-only, so that each row takes less horizontal space.
2. As a portal user on mobile, I want action controls to fit comfortably on small screens, so that I can launch tools without horizontal crowding.
3. As a portal user, I want official OpenCode and VS Code icons, so that actions are immediately recognizable.
4. As a portal user, I want a docs icon that matches the visual style, so that docs actions feel native to the UI.
5. As a portal user, I want icon buttons to keep descriptive tooltips, so that icon-only controls remain understandable.
6. As a keyboard user, I want icon buttons to have proper accessible labels, so that assistive technologies describe actions correctly.
7. As a portal user, I want running sessions to appear above the directory list, so that active context is visible first.
8. As a portal user, I want running sessions grouped by directory, so that I can reason about activity per workspace folder.
9. As a portal user, I want directory groups sorted alphabetically, so that I can find groups predictably.
10. As a portal user, I want session type buttons within each group in a stable order, so that my muscle memory remains reliable.
11. As a portal user, I want that stable order to match the directory action order, so that launching and managing sessions feels consistent.
12. As a portal user, I want running-session buttons to open the same URLs as today, so that behavior does not change unexpectedly.
13. As a portal user, I want session labels to show workspace-relative paths, so that the list is concise and easier to scan.
14. As a portal user, I want root-level sessions to show only the root directory name, so that I do not see long absolute paths.
15. As a portal user, I want starting sessions to show disabled action icons with a spinner, so that I can understand progress without accidental clicks.
16. As a portal user, I want one stop control per directory group, so that session-stop actions stay compact.
17. As a portal user, I want the stop control to show each stoppable session clearly, so that I can stop the exact one I intend.
18. As a portal user, I want stop actions to keep confirmation prompts, so that accidental session termination is less likely.
19. As a mobile user, I want the stop dropdown to be easy to tap, so that session control remains practical on touch devices.
20. As a maintainer, I want the UI redesign to preserve existing start/open/stop endpoints, so that backend contracts stay stable.
21. As a maintainer, I want SSE-driven updates to continue working with the grouped view, so that live session state remains accurate.
22. As a maintainer, I want deterministic sorting and labeling rules, so that rendering behavior is testable and predictable.
23. As a maintainer, I want icon assets to be bundled locally, so that rendering does not depend on external networks.
24. As a maintainer, I want this extension to remain additive and non-destructive, so that existing workflows continue to function.

## Implementation Decisions

- Introduce a session-grouping presentation module that transforms raw running sessions into a directory-grouped view model with:
  - deterministic directory label generation,
  - root-label exception handling,
  - alphabetical group ordering,
  - fixed per-group session-type ordering.
- Introduce a compact action descriptor module that maps each session type to:
  - icon asset identifier,
  - human-readable label,
  - visual variant metadata,
  - shared ordering priority.
- Keep session lifecycle ownership unchanged: starting, health transitions, dedupe, and stopping remain managed by the existing manager contracts.
- Replace text labels in directory actions with icon-only controls while retaining explicit accessibility metadata.
- Replace the flat session-row presentation with grouped directory sections that render:
  - open actions as icon buttons,
  - starting-state disabled icon buttons with spinner,
  - one per-directory stop dropdown listing active session targets.
- Keep stop behavior bound to session identity so selecting a dropdown item maps to the exact existing stop operation.
- Reorder the page composition so Running Sessions renders before Directories.
- Bundle icon assets with the application and serve them from the existing static-asset surface to avoid runtime third-party dependencies.
- Maintain consistency between launch and session action order using one canonical order rule across both surfaces.
- Preserve HTMX and SSE update flow, with grouped rendering returned through the same partial-update pathway.

## Testing Decisions

- A good test verifies externally observable behavior and user-visible outcomes, not template internals or incidental implementation details.
- Verify directory actions render icon-based controls with accessible labels and expected ordering.
- Verify running sessions render above the directory tree.
- Verify sessions are grouped by directory and groups are alphabetically ordered.
- Verify per-group session actions follow the shared canonical type order.
- Verify root directory label behavior differs from non-root relative-path rendering exactly as specified.
- Verify starting sessions render disabled open controls with a visible loading indicator state.
- Verify stop dropdown exposes correct per-session targets and preserves confirmation behavior.
- Verify existing start and stop operations still produce correct session lifecycle effects through unchanged routes.
- Verify SSE-triggered refreshes continue to render updated grouped state correctly.
- Prior art should follow existing server handler/rendering tests and session lifecycle tests in this repository where observable HTML fragments and request behaviors are already asserted.

## Out of Scope

- Changing session process management, startup commands, health polling logic, or persistence semantics.
- Introducing new session types or changing the meaning of existing types.
- Redesigning broader page branding, themes, or typography beyond what is needed for compact controls and grouped sessions.
- Adding fuzzy search, filtering, pinning, or custom sorting controls for directories or sessions.
- Reworking authentication, authorization, or network exposure behavior.
- Adding external icon CDNs or dynamic runtime icon fetching.

## Further Notes

- Mobile ergonomics are a first-class requirement for this extension; controls should remain tappable and readable under narrow widths.
- Consistency is intentional: users should see the same type order wherever actions appear.
- The root-directory label exception applies only to running-session path display, not to internal session identity.

---

# Plan: Compact Actions and Grouped Sessions UI

> Source PRD: Extension 04 - Compact Actions and Grouped Sessions UI

## Architectural decisions

Durable decisions that apply across all phases:

- **Routes**: Keep existing session and page routes unchanged (`/`, `/sessions`, `/sessions/start`, `/sessions/stop`, `/events`, static assets route).
- **Session lifecycle boundary**: Session creation, dedupe, health progression, URL publication, and stop semantics stay in the existing session manager contract.
- **Key view models**: Introduce stable presentation models for action descriptors and grouped running sessions; templates consume these models instead of raw session lists.
- **Directory label rules**: Running-session labels are workspace-relative, with a root exception that shows only the workspace root basename.
- **Ordering rules**: Directory groups sort alphabetically by rendered directory label; session actions use one canonical type order everywhere: OpenCode, VS Code, docs.
- **Asset boundary**: Icons are bundled and served locally from the app's static surface (no external CDN/runtime fetch).
- **Interaction model**: Open actions remain direct links; stop actions remain POST-driven by session identity with confirmation; live updates remain SSE + partial refresh.

---

## Phase 1: Icon Action Baseline

**User stories**: 1, 2, 3, 4, 5, 6, 23

### What to build

Deliver icon-only launch actions in the directory tree (including root row) using bundled local icons and accessible metadata, while preserving current start behavior.

### Acceptance criteria

- [ ] Directory launch actions render as icon-only controls for OpenCode, VS Code, and docs.
- [ ] Icons are served from locally bundled static assets.
- [ ] Icon controls keep clear `title` and accessible label metadata.
- [ ] Starting a session from any icon action still uses the existing start flow and succeeds as before.
- [ ] Touch targets remain usable on mobile widths.

---

## Phase 2: Session Labels and Section Priority

**User stories**: 7, 13, 14, 22

### What to build

Make the running-sessions area concise and prominent by moving it above the directory tree and replacing absolute paths with deterministic concise labels.

### Acceptance criteria

- [ ] Running Sessions renders above Directories in the page.
- [ ] Non-root sessions display workspace-relative directory labels.
- [ ] Root sessions display only the workspace root basename.
- [ ] Label rendering is deterministic and testable.

---

## Phase 3: Grouped Sessions with Open Icons

**User stories**: 8, 9, 10, 11, 12, 20, 21, 22, 24

### What to build

Replace the flat running-sessions list with directory-grouped sections. Each group shows per-type icon actions that open sessions exactly like the current Open action.

### Acceptance criteria

- [ ] Running sessions are grouped by directory label.
- [ ] Directory groups are sorted alphabetically by label.
- [ ] Session action order within every group is OpenCode, VS Code, docs.
- [ ] Clicking a running-session icon opens the same destination behavior as the current Open action.
- [ ] Existing endpoints and SSE-triggered refresh flow continue to work with grouped rendering.

---

## Phase 4: Starting-State Feedback in Grouped View

**User stories**: 15, 21, 22

### What to build

Add explicit in-progress feedback for sessions that are not yet openable: render disabled icon actions with spinner state in grouped sessions.

### Acceptance criteria

- [ ] Sessions without a ready URL render disabled icon actions.
- [ ] Disabled actions include visible spinner/loading feedback.
- [ ] Once healthy, actions become openable on refresh without extra user input.
- [ ] Starting-state behavior remains stable under SSE-driven updates.

---

## Phase 5: Per-Directory Stop Dropdown

**User stories**: 16, 17, 18, 19, 20, 24

### What to build

Introduce one stop control per directory group that expands to session-specific stop choices, preserving confirmation and current stop semantics.

### Acceptance criteria

- [ ] Each directory group provides a single stop dropdown control.
- [ ] Dropdown options identify each stoppable session clearly (type and distinguishing context).
- [ ] Selecting an option stops only the targeted session.
- [ ] Stop actions preserve confirmation prompts before termination.
- [ ] Stop control remains usable and tappable on mobile.

---

## Phase 6: End-to-End Regression and UX Hardening

**User stories**: 2, 6, 19, 20, 21, 22, 24

### What to build

Harden the full flow with regression coverage across rendering, interactions, ordering, and live updates to ensure the redesign remains additive and reliable.

### Acceptance criteria

- [ ] Automated coverage validates grouped rendering, ordering, label rules, and icon accessibility metadata.
- [ ] Automated coverage validates open and stop interactions continue to work through existing routes.
- [ ] Automated coverage validates SSE-driven refreshes render correct grouped state transitions.
- [ ] Existing OpenCode/VS/docs lifecycle behavior remains non-regressed.
