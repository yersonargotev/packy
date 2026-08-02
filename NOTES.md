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

- **Issue delivery**: on an explicit request for one approved issue, qualify and
  optionally diagnose it, implement with proportional local review, build and
  exercise the Packy binary manually, then open a ready PR for required CI and
  complete final Standards + Spec review before protected merge and cleanup.
- **Release**: publication of a verified `main` commit through the existing
  `release-packy` gate.
