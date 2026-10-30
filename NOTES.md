# Workspace Notes

## Packy development

- Source control and delivery use GitHub issues, pull requests, and Actions.
- Packy is a Go CLI; follow the
  [repository validation contract](README.md#verification) before delivery.
- Architecture and symbol discovery use CodeGraph before source inspection;
  runtime behavior is verified with real commands or tests.
- Tests and manual checks that exercise user configuration sandbox `HOME` and
  `XDG_CONFIG_HOME`.
- External project state such as issue labels, pull requests, merges, tags, and
  releases stays with the primary agent.
- `.agents/skills/release-packy` is the existing project-local release skill.

## Canonical loops

- **Development**: use the existing poteto-mode playbook with
  [Packy's verification and review policy](docs/agents/development.md).
  End with a ready PR for human merge. Agent integration needs a separate
  explicit Shipping request.
- **Release**: publication of a verified `main` commit through the existing
  `release-packy` gate.
