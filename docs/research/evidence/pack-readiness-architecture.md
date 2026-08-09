# Evidence: Pack readiness architecture

This note records primary-source evidence for the architecture approved in
[issue 617](https://github.com/yersonargotev/packy/issues/617) and delivered by
[issue 618](https://github.com/yersonargotev/packy/issues/618). It supports the
approved decisions; it does not define additional Pack behavior or contracts.

## Application-owned readiness policy and adapter-reported facts

Alistair Cockburn's original ports-and-adapters description defines a port as a
purposeful conversation and says that a technology-specific adapter converts an
external event into a procedure call or message for the application. It also
states that multiple adapters can serve one port, including an in-memory mock
used while testing the application in isolation. [Cockburn, *Hexagonal
Architecture*](https://alistair.cockburn.us/hexagonal-architecture)

Cockburn distinguishes a primary actor that drives the application from a
secondary actor that the application drives for answers or notifications.
[Cockburn, *Hexagonal
Architecture*](https://alistair.cockburn.us/hexagonal-architecture)

This supports the approved boundary: the capability-pack domain owns readiness
obligation evaluation and aggregation, while CLI, Doctor, structured output,
and TUI drive that use case, and surface adapters supply observations or perform
the reviewed host conversations. An in-memory observation adapter is therefore
an appropriate domain-test seam. It does **not** require ceremonial interfaces
where no real conversation boundary exists.

## Conditions preserve unknown state and its context

Kubernetes documents a condition as a named `type` with a `status` of `True`,
`False`, or `Unknown`; a condition also has probe/transition time, a
machine-readable `reason`, and a human-readable `message`. [Kubernetes, *Pod
Lifecycle: Pod conditions*](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions)

Kubernetes documents `observedGeneration` as the Pod generation at the time a
condition was recorded. [Kubernetes, *Pod
Conditions*](https://kubernetes.io/docs/concepts/workloads/pods/pod-condition/#fields-of-a-podcondition)

Kubernetes readiness gates require every specified condition to be `True` for
the Pod to be ready; a missing custom condition is defaulted to `False` when
the condition is evaluated. [Kubernetes, *Pod
Lifecycle: Pod readiness*](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-readiness)

These conventions support Packy's approved condition record: a stable
condition type, tri-state value, reason, message, evidence, observation time,
and validity identity. They support preserving `unknown` rather than treating
it as failure. Packy's configured/authorized/usable dimensions and its stated
false-dominates aggregation rule remain Packy decisions, rather than claims
about the Kubernetes API.

## Reviewed declarative capability vocabulary

GitHub's first-party GitHub App manifest flow uses a JSON-encoded manifest to
create a preconfigured registration. The manifest includes permissions, events,
and webhook configuration. [GitHub Docs, *Registering a GitHub App from a
manifest*](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest)

GitHub documents `default_events` as an array of subscribed events and
`default_permissions` as an object whose keys are permission names and whose
values are access types; its documented manifest parameter table assigns types
to the other configuration fields as well. [GitHub Docs, *GitHub App Manifest
parameters*](https://docs.github.com/en/apps/sharing-github-apps/registering-a-github-app-from-a-manifest#github-app-manifest-parameters)

This is first-party evidence for the approved manifest approach: declare
reviewable host capabilities as explicit, typed vocabulary in data, validate
unknown entries, and keep execution semantics in the host implementation. It
does not justify Pack-provided executable probes, arbitrary commands, or
untyped extension blobs; those remain expressly excluded by the approved
Packy design.

## Source boundaries

The sources establish architectural analogies and conventions, not a mandate to
copy their APIs. Packy's ADR and issue decisions remain the authority for its
names, schema, aggregation, storage, scope, and security boundaries.
