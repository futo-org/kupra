# Changelog

## [0.2.0](https://github.com/futo-org/kupra/compare/v0.1.0...v0.2.0) (2026-11-06)


### Features

* support CloudNativePG clusters as state ([#31](https://github.com/futo-org/kupra/issues/31)) ([89d13d2](https://github.com/futo-org/kupra/commit/89d13d297c815cc391f9d92aebc1ee287b425a5e))


### Bug Fixes

* cap PreviewPolicy names at 63 characters and give the operator time to release its lease ([#30](https://github.com/futo-org/kupra/issues/30)) ([2b4a9da](https://github.com/futo-org/kupra/commit/2b4a9dabcc223c315f030b63352e66639e5b6d65))
* delete restore copies with their snapshot ([#21](https://github.com/futo-org/kupra/issues/21)) ([bfcba51](https://github.com/futo-org/kupra/commit/bfcba514965aa2ba605d1e73ceb71b3ab2b6e3c7))
* keep reset requests when a GitOps tool owns the Preview ([#25](https://github.com/futo-org/kupra/issues/25)) ([bd64ac1](https://github.com/futo-org/kupra/commit/bd64ac125ef8eb7c716c7213efa8aaddcd2f0df6))
* read a preview's namespace live when deleting it ([#26](https://github.com/futo-org/kupra/issues/26)) ([8107454](https://github.com/futo-org/kupra/commit/8107454f51f5e0a76f6fa1eff040e43e5eeb4b14))
* record parameter defaults in a snapshot's parameters ([#22](https://github.com/futo-org/kupra/issues/22)) ([418b660](https://github.com/futo-org/kupra/commit/418b660def12566a219153641b08f7e63fb881a6))
* refuse template settings and names that expressions can't use ([#27](https://github.com/futo-org/kupra/issues/27)) ([b09319c](https://github.com/futo-org/kupra/commit/b09319cdd79122da13477d45dc4d398d4b742b54))
* report waits that clear by themselves as reconciling, not stalled ([#29](https://github.com/futo-org/kupra/issues/29)) ([61ead6e](https://github.com/futo-org/kupra/commit/61ead6e60cca3a1b8e8373957e702202f94cafd0))
* require Kubernetes 1.36 in the chart ([#28](https://github.com/futo-org/kupra/issues/28)) ([f6cf47b](https://github.com/futo-org/kupra/commit/f6cf47bc3750edffdfc8f4bac86eaa328f733d73))

## 0.1.0 (2026-11-05)


### Features

* install kupra with a Helm chart ([#11](https://github.com/futo-org/kupra/issues/11)) ([c7d39a2](https://github.com/futo-org/kupra/commit/c7d39a272bc37866a970ddf3189249c38f4b1828))
* PersistentVolumeClaims as state, with snapshots, restore and reset ([#12](https://github.com/futo-org/kupra/issues/12)) ([f31fd6e](https://github.com/futo-org/kupra/commit/f31fd6e1c69c650e95b78248eddf5ae4e4bd5c82))
* previews without state ([#5](https://github.com/futo-org/kupra/issues/5)) ([79fef72](https://github.com/futo-org/kupra/commit/79fef72d5d17b9a020710d62fae91962edcd02d8))
