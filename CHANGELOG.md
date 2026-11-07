# Changelog

## [0.3.0](https://github.com/futo-org/kupra/compare/v0.2.0...v0.3.0) (2026-11-07)


### Features

* add the dashboard ([#57](https://github.com/futo-org/kupra/issues/57)) ([23474b3](https://github.com/futo-org/kupra/commit/23474b3ecca1a4a0e58085c2ecbcc14d880b5f09))
* label every object the chart templates, and validate the chart's values ([#47](https://github.com/futo-org/kupra/issues/47)) ([d03b6f1](https://github.com/futo-org/kupra/commit/d03b6f106f4f17338b48924cccae5500564c0b5d))
* label the namespaces that contain PreviewApps ([#55](https://github.com/futo-org/kupra/issues/55)) ([091e416](https://github.com/futo-org/kupra/commit/091e41617aa768fccb8c05d0b87227befd0725eb))
* report on each PreviewPolicy whether previews can use it ([#45](https://github.com/futo-org/kupra/issues/45)) ([1e05efc](https://github.com/futo-org/kupra/commit/1e05efcff1f2aea9d6ec1fe11091ce3fe56b7934))
* show readiness and status in kubectl get ([#35](https://github.com/futo-org/kupra/issues/35)) ([405dc9c](https://github.com/futo-org/kupra/commit/405dc9c0447d1c1ec681931125d13be123ff6466))


### Bug Fixes

* create restored claims and clusters once their copies are bound ([#39](https://github.com/futo-org/kupra/issues/39)) ([b7761c6](https://github.com/futo-org/kupra/commit/b7761c640d8d683bfbb3a3456a85dd6e5f5b628a))
* **dashboard:** show a new preview's actions to its owners, and hide namespaces without visible apps ([#61](https://github.com/futo-org/kupra/issues/61)) ([4baf1b7](https://github.com/futo-org/kupra/commit/4baf1b7cb8e144429b541b96121cbfa57902a6e3))
* keep the app-namespace label empty ([#56](https://github.com/futo-org/kupra/issues/56)) ([639ee53](https://github.com/futo-org/kupra/commit/639ee5378b2dc3dfd3f1b6d2f9122589aa2489ef))
* record a preview's generation as observed only once its owners are bound ([#62](https://github.com/futo-org/kupra/issues/62)) ([471dc7b](https://github.com/futo-org/kupra/commit/471dc7bccbfbc297a9f4b4f91a36000d0153f964))
* report render errors as field errors ([#51](https://github.com/futo-org/kupra/issues/51)) ([a2a159c](https://github.com/futo-org/kupra/commit/a2a159c922214752ddb19cab49a454f810edc7d2))
* say why in the Status column while capturing, deleting and resetting ([#41](https://github.com/futo-org/kupra/issues/41)) ([c6155f1](https://github.com/futo-org/kupra/commit/c6155f1057afc908c70063c668e6bbec1f0d8cb7))

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
