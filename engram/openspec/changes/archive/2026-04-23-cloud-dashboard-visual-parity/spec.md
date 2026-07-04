# Delta Spec: cloud-dashboard (cloud-dashboard-visual-parity)

[See /Users/alanbuscaglia/work/engram/openspec/specs/cloud-dashboard/spec.md for the merged main specification]

This delta spec contains 14 new requirements (REQ-100 through REQ-113) that extend the base cloud-dashboard specification to include full visual and interaction parity with the legacy engram-cloud dashboard.

Key additions:
- REQ-100: Static asset byte-size floors (40 KB htmx, 60 KB pico, 20 KB styles)
- REQ-101: Templ deterministic regeneration and version pin to v0.3.1001
- REQ-102: Row type extensions with detail fields (Content, TopicKey, ToolName, etc.)
- REQ-103: MountConfig GetDisplayName optional field with "OPERATOR" fallback
- REQ-104: Project sync control persistence via cloud_project_controls table
- REQ-105: Dashboard system health aggregator
- REQ-106: Full HTMX endpoint surface (11 endpoints)
- REQ-107: Layout structural parity (ribbon, hero, tabs, footer)
- REQ-108: HTMX attribute presence per endpoint
- REQ-109: Push-path pause enforcement with 409 Conflict response
- REQ-110: Insecure-mode login regression guard
- REQ-111: Copy parity strings rendered in HTML
- REQ-112: Admin gate on toggle mutations (403 for non-admin)
- REQ-113: Principal bridge for identity mapping (no direct context reads)

All requirements are testable via go test without a browser. Test seams map 1:1 to the specification requirements.

For full details, see the main spec at openspec/specs/cloud-dashboard/spec.md.
