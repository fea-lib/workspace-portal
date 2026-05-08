---
title: "Workspace Portal — Course Index"
---

# Workspace Portal — Course Index

A self-study series for building **workspace-portal** from scratch. Each course produces working code and teaches the technology used to build the portal.

**Audience:** Developers familiar with TypeScript/JavaScript who want to learn Go and HTMX through a real project.

**Project:** A self-hosted, mobile-friendly web portal that launches and manages per-directory [OpenCode](https://opencode.ai), [code-server](https://github.com/coder/code-server), and npm script sessions.

---

## Courses

| Course | Topic | What you build |
|---|---|---|
| [Course 01](./course-01-go-foundations.md) | Go Foundations | Go language, stdlib, toolchain — referenced throughout |
| [Course 02](./course-02-building-the-portal.md) | Building the Portal in Go | All Go modules: config, fs, session (oc, vscode, script), HTTP server (text stubs) |
| [Course 03](./course-03-htmx-and-sse.md) | HTMX and Server-Sent Events | Full UI: directory tree, session management, Scripts dialog picker, live SSE updates |
| [Course 04](./course-04-script-runner.md) | Script Runner | End-to-end script runner: `package.json` detection, dialog picker, session wiring |
| [Course 05](./course-05-deployment.md) | Deployment | launchd service, production checklist, README, config example |
| [Course 06](./course-06-tailscale.md) | Tailscale Setup | Install, MagicDNS + HTTPS in admin console, `tailscale serve`, `internal/tailscale` Go module |

---

## Prerequisites

- Comfortable with TypeScript/JavaScript (used as the comparison language throughout)
- A Mac with Go 1.22+ installed

---

## Learning Path

The courses are designed to be read and coded in order. Each course begins where the previous one left off. If you are already comfortable with a topic, you can skim the early lessons and start coding at the first code block you have not seen before.

---

## Reference

- [PRD — Product Requirements Document](./00-prd.md) — full specification: user stories, module design, config schema, SSE events, testing decisions
- [Go standard library](https://pkg.go.dev/std)
- [HTMX documentation](https://htmx.org/docs/)
- [launchd reference](https://developer.apple.com/library/archive/documentation/MacOSX/Conceptual/BPSystemStartup/Chapters/CreatingLaunchdJobs.html)
