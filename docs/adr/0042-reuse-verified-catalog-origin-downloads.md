---
status: accepted
---

# Reuse verified Catalog Project origin downloads

## Decision

Catalog authoring retains exact-origin checkouts in
`os.UserCacheDir()/packy/catalog-origins-v1`, separately from installed Catalog
Snapshots and host configuration. The approved issue #828 delegates the location,
retention, concurrency, and integrity choices to this implementation.

An entry is keyed by the SHA-256 of the exact repository name, a NUL separator,
and full commit. Acquisition still clones the authoritative public GitHub origin
with history and tags and checks out that exact commit. No branch substitution,
validation verdict caching, or automatic transport downgrade is allowed.

A single advisory directory lock serializes acquisition, copying, and eviction.
It follows the repository's existing context-aware catalog transaction locking
convention. This intentionally simple coarse lock also bounds concurrent disk
and network work. Each resolution has a five-minute deadline, including lock
acquisition. Complete checkouts are sealed with a SHA-256 tree digest covering
paths, types, permissions, regular-file bytes, and symlink targets, including
Git metadata. After the private consumer copy is verified and cancellation is
checked, the complete entry is atomically renamed into place. Incomplete staging directories
are removed on error or on the next locked resolution after interruption.

Every invocation receives a private temporary copy, whose digest must match the
seal before it is returned. Resolvers never expose retained cache paths to
consumers. Corruption is rejected with the exact entry path; there is no automatic
repair retry. An operator may remove the named entry while authoring commands
are idle and retry acquisition from the same authoritative origin. The cache is
private to the OS user; the seal detects accidental modification and corruption,
not an attacker who can rewrite both retained data and its seal as that user.

On each resolution, entries unused for seven days are removed and least recently
used entries are evicted to keep at most eight complete entries. Active consumers
use their own temporary copies, so eviction cannot invalidate their work. Idle
caches do not grow. The entire named cache directory may be deleted while
all authoring commands are idle; the next resolution reconstructs it.

## Consequences

Warm invocations avoid another full upstream download but still copy and verify
source evidence locally and fully validate the current candidate and baseline.
Cold-download transport failures remain possible. Retention is bounded by entry
count and age, not a byte quota; one upstream repository may be large. This does
not change catalog schemas, publication authority, or installed snapshot
retention. Exact-origin content remains inert, including Git hooks.
