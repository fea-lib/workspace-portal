---
title: "Extension: Sleep Resilience"
---

# Extension: Sleep Resilience with caffeinate

## Problem Statement

When the workspace portal is running on macOS, closing the laptop lid can make the portal process and editor sessions effectively unavailable from the network. From a user perspective, this breaks the expectation that the portal is an always-on service reachable through local or Tailnet URLs.

Today, users can start the portal and launch OpenCode and code-server sessions, but the runtime does not consistently hold a sleep assertion for the portal service and child sessions. As a result, long-running sessions may become unreachable until the machine wakes again.

## Solution

Add explicit sleep-resilience behavior using `caffeinate` at two levels:

1. Service level: run the persistent macOS service through `caffeinate -s` so the always-on portal process carries a system sleep assertion while it is active.
2. Session level: when launching OpenCode and code-server sessions, attach a best-effort `caffeinate -s -w <pid>` helper to each session process, so each session keeps the machine awake while it is alive.

Also update deployment and course documentation so users understand that the launchd service is caffeinate-wrapped, how this affects reliability, and how to validate the behavior after install/reload.

## User Stories

1. As a portal operator, I want the service process to run with a sleep assertion, so that the portal stays reachable during normal unattended usage.
2. As a remote user, I want existing portal URLs to remain responsive after my laptop lid has been closed and reopened, so that I do not lose access to active work.
3. As an OpenCode user, I want sessions launched from the portal UI to hold sleep assertions while running, so that active coding sessions do not disappear unexpectedly.
4. As a code-server user, I want editor sessions started by the portal to remain available as background services, so that browser tabs keep working across inactivity windows.
5. As a macOS user, I want the launchd-installed portal to include sleep behavior out of the box, so that I do not need ad hoc shell wrappers.
6. As a user reinstalling the service, I want the generated service definition to include caffeinate automatically, so that redeploying does not regress reliability.
7. As a user following the course docs, I want deployment snippets to match real runtime behavior, so that setup instructions stay trustworthy.
8. As a maintainer, I want course documentation to explain why caffeinate is used, so that design intent is obvious to future contributors.
9. As a maintainer, I want the service and session sleep strategy documented in troubleshooting guidance, so that availability issues are easier to diagnose.
10. As a user on macOS, I want session startup to succeed even if caffeinate is unavailable or fails, so that core editor launch behavior remains functional.
11. As a maintainer, I want failure to start caffeinate helpers to be non-fatal and visible in logs, so that reliability degrades gracefully instead of hard failing.
12. As a user, I want session lifecycle behavior to stay unchanged (start, health check, stop, cleanup), so that adding caffeinate does not break existing workflows.
13. As an operator, I want stop actions to continue terminating sessions cleanly, so that child sleep helpers do not leave orphaned process noise.
14. As a maintainer, I want process ownership and PID tracking semantics preserved, so that existing manager logic and state persistence remain stable.
15. As a deployer, I want install and reload workflows to keep working with no additional manual edits, so that rollout remains one-command.
16. As a user, I want logs to show enough signal when caffeinate cannot be started, so that I can correct PATH or platform issues quickly.
17. As a reviewer, I want implementation boundaries to stay modular, so that service-level and session-level sleep behavior can evolve independently.
18. As a QA engineer, I want clear post-deploy checks for sleep-resilience behavior, so that regressions are caught before daily use.
19. As a documentation reader, I want explicit notes about expected behavior and limitations on closed-lid scenarios, so that operational expectations are realistic.
20. As a project maintainer, I want this extension to preserve existing UX and endpoint contracts, so that no client integrations break.

## Implementation Decisions

- Use a two-layer sleep strategy:
  - **Service layer** for baseline always-on portal availability.
  - **Session layer** for per-session resilience when sessions are spawned by the portal.
- Keep process lifecycle interfaces stable by preserving direct ownership of the editor process in the session manager.
- Attach session-level caffeinate as a helper process waiting on the editor PID, rather than replacing the launched command with a wrapper process.
- Treat caffeinate as best-effort at session-launch time:
  - session launch remains successful even if caffeinate fails.
  - helper startup failure is logged for diagnosis.
- Make launchd generation include caffeinate in program arguments so newly installed or reinstalled services are correct by default.
- Keep the deployment script contract unchanged for users (same install/reload command flow), while generated output reflects the new launch strategy.
- Update course and user-facing docs to align with runtime behavior, including rationale and validation guidance.
- Preserve all existing portal routes, session APIs, and UI actions; this extension is reliability-focused, not feature-surface-changing.

## Testing Decisions

- A good test verifies externally observable behavior and lifecycle outcomes, not private implementation details.
- Verify service definition generation includes the caffeinate invocation and expected argument ordering.
- Verify session launch still succeeds when helper startup is skipped or fails, confirming graceful degradation.
- Verify session stop/cleanup behavior remains correct with sleep helpers present (no session lifecycle regressions).
- Verify logging occurs for non-fatal helper startup failures, so operational diagnostics remain available.
- Verify documentation examples are consistent with generated deployment artifacts and runtime assumptions.
- Prior art should follow existing patterns for session manager lifecycle tests, process-launch behavior tests, and deployment template/install script validation patterns already used in this codebase.

## Out of Scope

- Non-macOS deployment targets and supervisor integrations.
- Replacing launchd with another service manager.
- Adding new authentication or authorization controls.
- Changing session URL schemes, routing, or UI controls beyond what is needed for documentation accuracy.
- Building a full power-management policy engine beyond targeted `caffeinate` integration.
- Guaranteeing behavior that macOS power policy explicitly disallows in every hardware/lid state scenario.

## Further Notes

- This extension is a reliability enhancement focused on preserving reachability and continuity.
- The deployment step should include reinstall/reload of the user agent so the updated service definition is applied.
- Operator-facing docs should include quick verification steps (service status, active session availability, and process/log checks) after rollout.

## Vertical Slice Issues (Drafts)

These are independently grabbable tracer-bullet slices derived from this PRD. All slices are marked AFK because they can be implemented and reviewed without a blocking human design decision.

### 1) Service Sleep Assertion via launchd

**Type:** AFK  
**Blocked by:** None - can start immediately  
**User stories covered:** 1, 2, 5, 6, 15, 18

## Parent PRD

`docs/extensions/02-sleep-resilience-caffeinate-prd.md`

## What to build

Update the generated macOS launch agent command so the portal service runs through `caffeinate -s` in production installs. Keep install/reload workflow unchanged for operators.

## Acceptance criteria

- [ ] Generated launchd service arguments invoke `caffeinate -s` before the portal binary.
- [ ] Installing or reinstalling the service applies the updated service definition without extra manual steps.
- [ ] The service still starts successfully through the existing install flow.

## Blocked by

None - can start immediately.

## User stories addressed

- User story 1
- User story 2
- User story 5
- User story 6
- User story 15
- User story 18

### 2) OpenCode Session-Level Caffeinate Helper

**Type:** AFK  
**Blocked by:** None - can start immediately  
**User stories covered:** 3, 10, 11, 12, 14, 16, 17

## Parent PRD

`docs/extensions/02-sleep-resilience-caffeinate-prd.md`

## What to build

Introduce a reusable session-level helper that attaches `caffeinate -s -w <pid>` to launched editor processes, then integrate it with OpenCode session startup as a best-effort behavior.

## Acceptance criteria

- [ ] OpenCode sessions start a `caffeinate -s -w <pid>` helper after successful process launch.
- [ ] If helper startup fails, session startup remains successful and logs a non-fatal warning.
- [ ] Existing OpenCode start/health lifecycle behavior remains unchanged.

## Blocked by

None - can start immediately.

## User stories addressed

- User story 3
- User story 10
- User story 11
- User story 12
- User story 14
- User story 16
- User story 17

### 3) VS Code Session-Level Caffeinate Integration

**Type:** AFK  
**Blocked by:** 2) OpenCode Session-Level Caffeinate Helper  
**User stories covered:** 4, 10, 11, 12, 13, 14, 16, 20

## Parent PRD

`docs/extensions/02-sleep-resilience-caffeinate-prd.md`

## What to build

Apply the same reusable session-level caffeinate helper to code-server session startup and verify lifecycle behavior remains stable and API/UI flows remain unchanged.

## Acceptance criteria

- [ ] code-server sessions attach a `caffeinate -s -w <pid>` helper after launch.
- [ ] Stop and cleanup behavior remains correct for code-server sessions.
- [ ] Existing user-facing session flows and endpoints are unchanged.

## Blocked by

- Blocked by issue "OpenCode Session-Level Caffeinate Helper".

## User stories addressed

- User story 4
- User story 10
- User story 11
- User story 12
- User story 13
- User story 14
- User story 16
- User story 20

### 4) Reliability Regression Tests for Sleep Strategy

**Type:** AFK  
**Blocked by:** 1) Service Sleep Assertion via launchd, 2) OpenCode Session-Level Caffeinate Helper, 3) VS Code Session-Level Caffeinate Integration  
**User stories covered:** 11, 12, 13, 14, 16, 18, 20

## Parent PRD

`docs/extensions/02-sleep-resilience-caffeinate-prd.md`

## What to build

Add or extend tests that validate externally observable behavior for service argument generation, non-fatal helper failure handling, and unchanged session lifecycle behavior.

## Acceptance criteria

- [ ] Automated coverage verifies service definition includes caffeinate invocation.
- [ ] Automated coverage verifies non-fatal behavior when session-level helper startup fails.
- [ ] Automated coverage verifies session stop/cleanup behavior remains correct.

## Blocked by

- Blocked by issue "Service Sleep Assertion via launchd".
- Blocked by issue "OpenCode Session-Level Caffeinate Helper".
- Blocked by issue "VS Code Session-Level Caffeinate Integration".

## User stories addressed

- User story 11
- User story 12
- User story 13
- User story 14
- User story 16
- User story 18
- User story 20

### 5) Docs Alignment: Deployment, Course, and Troubleshooting

**Type:** AFK  
**Blocked by:** 1) Service Sleep Assertion via launchd, 2) OpenCode Session-Level Caffeinate Helper, 3) VS Code Session-Level Caffeinate Integration  
**User stories covered:** 7, 8, 9, 15, 18, 19

## Parent PRD

`docs/extensions/02-sleep-resilience-caffeinate-prd.md`

## What to build

Update user-facing and course documentation so deployment snippets and operational guidance accurately describe the launchd caffeinate wrapper and session-level best-effort caffeinate behavior.

## Acceptance criteria

- [ ] Deployment/course docs show the updated launchd strategy and rationale.
- [ ] Troubleshooting guidance includes caffeinate-related diagnostics and expectations.
- [ ] Post-deploy verification steps are documented for operators.

## Blocked by

- Blocked by issue "Service Sleep Assertion via launchd".
- Blocked by issue "OpenCode Session-Level Caffeinate Helper".
- Blocked by issue "VS Code Session-Level Caffeinate Integration".

## User stories addressed

- User story 7
- User story 8
- User story 9
- User story 15
- User story 18
- User story 19
