# TUI Pack and resource controls

Status: confirmed for implementation.

## Confirmed requirements

- Users can manage individual Packs and their resource selections from the TUI
  opened by `packy`.
- Support both global and project scopes, respecting their existing lifecycle
  distinctions and supported CLI surfaces.
- Support selecting only some skills/resources when installing or activating a
  Pack, changing that selection later, and deactivating a complete Pack.
- Global whole-Pack deactivation has the same semantics as
  `packy deactivate <pack>`. No additional remembered-selection behavior has
  been requested.
- Controls operate on individual Packs; a bulk operation across all Packs is
  not requested.
- When disabling a resource needed by another selected resource, propose
  disabling its consumers too and show the resulting changes before applying.
- Show the current resource selection and stage multiple changes before an
  explicit Apply changes action, preview, consent, and application.
- Clearing the global resource selection means whole-Pack deactivation; show
  that consequence before application.
- Expose project resource configuration separately from personal project
  activation and deactivation, preserving the existing lifecycle distinction.
- Clearing the project's resource selection means uninstalling that Pack from
  the project, with an explicit preview. Personal project deactivation preserves
  the project installation.
- Supporting assets and legal notices remain visible, but follow the resources
  that require them instead of offering independent activation controls.

## Interaction

Open a Pack with Enter, then open its lifecycle actions. Choose Configure
resources for an active global Pack or Configure project resources for an
installed project Pack. For a new activation or installation, choose Full Pack
or Choose resources.

Use the arrow keys to focus a resource and Space or Enter to toggle it. Tab
focuses Apply changes, which creates a preview; the existing consent flow must
complete before any mutation. Esc leaves the selection without applying it.
Left and right change CLI surface and load that surface's independent state.

Resource configuration uses the current reviewed catalog and update lifecycle,
including its version transition when the installed version differs. Preview
shows the selected version and effects before consent. Whole removal and
personal project activation retain their existing CLI semantics.

## Architectural context

[ADR 0033](../adr/0033-make-the-tui-the-primary-interactive-interface.md)
keeps the TUI as an adapter over the same domain lifecycle as the CLI, with
preview, consent, application, and verification for one Pack, surface, and
scope. This design retains that boundary. No new architectural decision has
been accepted during this interview.
