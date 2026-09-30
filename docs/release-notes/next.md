# {{TAG}} — Reliable aliased activation

This patch fixes approved Pack activation with a surface alias incorrectly
rejecting an unchanged catalog as stale.

## Changes since the previous release

The previous stable release is v0.2.24.

- An applicable, approved aliased activation now applies and verifies its
  intended projection. For example, Emil can use `emil-prototype` on Codex
  while Matty retains its existing `prototype` projection and receipt.
- All and custom resource selection, alias composition, and lifecycle contract
  presentation preserve the original catalog contract and the unaliased
  contract sealed for freshness validation.
- Real catalog-contract or activation-state changes after Preview still reject
  Apply before any projection action or activation-state write. Consent,
  ownership, and collision validation remain in place.

Catalog Snapshot and Pack manifest schemas remain v2. The release artifact
format is unchanged. Upgrading from v0.2.24 does not require catalog adoption
or reinstalling active Packs.

## Install or upgrade

```sh
brew upgrade yersonargotev/tap/packy
packy activate emil --surface codex --alias skill:prototype=emil-prototype
```

New installations may use `brew install yersonargotev/tap/packy`, then
`packy init` to acquire the current Catalog Snapshot. Claude Code **2.1.203 or
newer** remains the supported floor.
