# Engineering principles

Apply these defaults unless a more specific user or project instruction governs the work.

- Choose the simplest implementation that fully satisfies the known requirements.
- Build in small, end-to-end increments and keep the product working after each meaningful change.
- Give each component cohesive ownership. Add a boundary, layer, or module only when it provides a concrete separation benefit.
- Before implementing common functionality, inspect the existing dependencies, documentation, and types. Prefer an established, well-maintained library when it reduces total complexity or materially improves reliability.
- Design for every known requirement without planning a later replacement. When requirements remain uncertain, choose a simple, reversible decision.
- Remove obsolete and dead paths as part of the requested change. Keep compatibility layers only when compatibility is explicit.
- Follow the project's explicit compatibility policy. Otherwise, preserve public behavior, persisted data, and external contracts unless the task authorizes breaking them.
