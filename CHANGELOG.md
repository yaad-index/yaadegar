# Changelog

## [0.3.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.2.0...yaadegar-v0.3.0) (2026-07-29)


### Features

* email_confirmed reserve + confirm endpoint + email ([#81](https://github.com/yaad-index/yaadegar/issues/81)) ([#85](https://github.com/yaad-index/yaadegar/issues/85)) ([8404dac](https://github.com/yaad-index/yaadegar/commit/8404dacf8fc65434a25f8046418d246b4cc34466))
* reserver-tier foundations + confirm-window expiry ([#81](https://github.com/yaad-index/yaadegar/issues/81)) ([#83](https://github.com/yaad-index/yaadegar/issues/83)) ([b4d65cd](https://github.com/yaad-index/yaadegar/commit/b4d65cd22f2840f2bca0ee1cd8fccb132e22a476))

## [0.2.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.1.0...yaadegar-v0.2.0) (2026-07-29)


### Features

* auto-fill a new item from a pasted product URL ([#11](https://github.com/yaad-index/yaadegar/issues/11)) ([#79](https://github.com/yaad-index/yaadegar/issues/79)) ([2af73d4](https://github.com/yaad-index/yaadegar/commit/2af73d4a4a7681bdc62eb659114324a9a18a53c9))
* containerize the web frontend + full-stack compose ([#11](https://github.com/yaad-index/yaadegar/issues/11)) ([#74](https://github.com/yaad-index/yaadegar/issues/74)) ([9a2dec1](https://github.com/yaad-index/yaadegar/commit/9a2dec143c1c95504ebfd12c265dec9a3805d4f7))
* item editor with rich display — url/note fields, thumbnails, safe notes ([#11](https://github.com/yaad-index/yaadegar/issues/11)) ([#78](https://github.com/yaad-index/yaadegar/issues/78)) ([0051fa5](https://github.com/yaad-index/yaadegar/commit/0051fa5a5e9514da50ae39ab7396bbac0aa3ec37))


### Bug Fixes

* make the co-buying confirm transition atomic under the item lock ([#36](https://github.com/yaad-index/yaadegar/issues/36)) ([#76](https://github.com/yaad-index/yaadegar/issues/76)) ([49fe90b](https://github.com/yaad-index/yaadegar/commit/49fe90b6aa8f0516507f8fac4938193e74a1bc34))
* reclaim expired unverified custom-domain claims at add time ([#42](https://github.com/yaad-index/yaadegar/issues/42)) ([#77](https://github.com/yaad-index/yaadegar/issues/77)) ([1a168d7](https://github.com/yaad-index/yaadegar/commit/1a168d707ac228c772b6ecbf233f084af34ff9dc))
* URL-preview must not fail on an empty name ([#11](https://github.com/yaad-index/yaadegar/issues/11)) ([#80](https://github.com/yaad-index/yaadegar/issues/80)) ([d0bd2f0](https://github.com/yaad-index/yaadegar/commit/d0bd2f01d488ca3cc43ad18e21062c5a8e814893))

## 0.1.0 (2026-07-29)


### Features

* admin provisioning API — create tenant/owner over HTTP ([#30](https://github.com/yaad-index/yaadegar/issues/30) Cut A2b-2) ([#63](https://github.com/yaad-index/yaadegar/issues/63)) ([f060dd3](https://github.com/yaad-index/yaadegar/commit/f060dd3050072bb5024c7052c2bcb4f38d76eb80))
* atomic capacity primitive for reservations ([#33](https://github.com/yaad-index/yaadegar/issues/33)) ([#34](https://github.com/yaad-index/yaadegar/issues/34)) ([77580a3](https://github.com/yaad-index/yaadegar/commit/77580a38a8267cda82c64f61cfa1cfd2d237f665))
* browser auto-add from a product URL with SSRF-safe fetch ([#10](https://github.com/yaad-index/yaadegar/issues/10)) ([#41](https://github.com/yaad-index/yaadegar/issues/41)) ([88b115c](https://github.com/yaad-index/yaadegar/commit/88b115c55b14b9ed5140c4876721c9f29f754a18))
* core lists + items CRUD handlers ([#5](https://github.com/yaad-index/yaadegar/issues/5)) ([#31](https://github.com/yaad-index/yaadegar/issues/31)) ([2b4e5d5](https://github.com/yaad-index/yaadegar/commit/2b4e5d581487fc772cfeb0ffe80e4b60ace27293))
* event-dated lists auto-disable on the giver surface ([#9](https://github.com/yaad-index/yaadegar/issues/9)) ([#40](https://github.com/yaad-index/yaadegar/issues/40)) ([36c9d91](https://github.com/yaad-index/yaadegar/commit/36c9d91abe6b662d332c76ff65f224046c3954ff))
* frontend foundations — ADR-0006 + SvelteKit scaffold ([#11](https://github.com/yaad-index/yaadegar/issues/11) Cut F1) ([#67](https://github.com/yaad-index/yaadegar/issues/67)) ([1c07e65](https://github.com/yaad-index/yaadegar/commit/1c07e65d74682594370471b218a70340b754bcc3))
* generate API types + strict server from openapi.yaml ([#16](https://github.com/yaad-index/yaadegar/issues/16)) ([#18](https://github.com/yaad-index/yaadegar/issues/18)) ([d452536](https://github.com/yaad-index/yaadegar/commit/d452536db163231999bd786a3632021aea9d3aa4))
* Go project skeleton ([#2](https://github.com/yaad-index/yaadegar/issues/2)) ([#14](https://github.com/yaad-index/yaadegar/issues/14)) ([f2dadd9](https://github.com/yaad-index/yaadegar/commit/f2dadd9176a1fa09007cf5f4ee7d795c4d8f366d))
* group co-buying with email-handshake matching ([#7](https://github.com/yaad-index/yaadegar/issues/7)) ([#35](https://github.com/yaad-index/yaadegar/issues/35)) ([99dee96](https://github.com/yaad-index/yaadegar/commit/99dee96d23f07026b50cf0937ac738d30038cc22))
* list ownership via a join table with owner authorization ([#30](https://github.com/yaad-index/yaadegar/issues/30) Cut A2a) ([#59](https://github.com/yaad-index/yaadegar/issues/59)) ([90cedf7](https://github.com/yaad-index/yaadegar/commit/90cedf73f35bac661e26a961b5379fedfda5c5eb))
* local Docker setup for hands-on testing ([#55](https://github.com/yaad-index/yaadegar/issues/55)) ([#56](https://github.com/yaad-index/yaadegar/issues/56)) ([bcf0d25](https://github.com/yaad-index/yaadegar/commit/bcf0d256f4c063c9e40486eccb5a6ec2614395c7))
* login hardening — brute-force rate limit + timing equalization ([#54](https://github.com/yaad-index/yaadegar/issues/54), [#62](https://github.com/yaad-index/yaadegar/issues/62)) ([#64](https://github.com/yaad-index/yaadegar/issues/64)) ([20a8fbd](https://github.com/yaad-index/yaadegar/commit/20a8fbd08abe4ff4d89ec552d64762fa321183e9))
* multi-tenant custom domains + verification ([#12](https://github.com/yaad-index/yaadegar/issues/12)) ([#43](https://github.com/yaad-index/yaadegar/issues/43)) ([c6ad5a8](https://github.com/yaad-index/yaadegar/commit/c6ad5a8167fab3290f7aecef19b64d4eeedfa7a3))
* owner auth core — JWT + password login + fail-closed startup ([#30](https://github.com/yaad-index/yaadegar/issues/30) Cut A1) ([#53](https://github.com/yaad-index/yaadegar/issues/53)) ([9428bab](https://github.com/yaad-index/yaadegar/commit/9428baba9f85ea94f27582f799f4b7afd3c5b0b9))
* owner web flow — login, dashboard, list & item CRUD ([#11](https://github.com/yaad-index/yaadegar/issues/11) F2) ([#68](https://github.com/yaad-index/yaadegar/issues/68)) ([a74f16e](https://github.com/yaad-index/yaadegar/commit/a74f16e159f9d5aec6e25465cf5c93a09bc14c76))
* pluggable storage layer with structural tenant isolation ([#4](https://github.com/yaad-index/yaadegar/issues/4)) ([#17](https://github.com/yaad-index/yaadegar/issues/17)) ([d3876b5](https://github.com/yaad-index/yaadegar/commit/d3876b543749d41f50c9efacf8c5aa2a03b0472c))
* public giver flow — view, reserve, release ([#11](https://github.com/yaad-index/yaadegar/issues/11) F3) ([#70](https://github.com/yaad-index/yaadegar/issues/70)) ([d31d1fc](https://github.com/yaad-index/yaadegar/commit/d31d1fc7a49a8c2bc1487ba0743bca7cab9b442d))
* real SMTP email sender + decay send-gate ([#37](https://github.com/yaad-index/yaadegar/issues/37)) ([#44](https://github.com/yaad-index/yaadegar/issues/44)) ([39d28d8](https://github.com/yaad-index/yaadegar/commit/39d28d858ead8facb750c49202f754d5d1fb4c3e))
* reservation-decay engine ([#8](https://github.com/yaad-index/yaadegar/issues/8)) ([#38](https://github.com/yaad-index/yaadegar/issues/38)) ([3650883](https://github.com/yaad-index/yaadegar/commit/3650883f2efde2c0363ffba7e455a3e890000aba))
* reservations + reserver anonymity ([#6](https://github.com/yaad-index/yaadegar/issues/6)) ([#32](https://github.com/yaad-index/yaadegar/issues/32)) ([84144fa](https://github.com/yaad-index/yaadegar/commit/84144fa30101ba54c4fc3f980a3b5dc9f15602ca))
* superadmin surface + config bootstrap ([#30](https://github.com/yaad-index/yaadegar/issues/30) Cut A2b-1) ([#61](https://github.com/yaad-index/yaadegar/issues/61)) ([7dff3ef](https://github.com/yaad-index/yaadegar/commit/7dff3efea3912aec76ff673df20232e2643f7868))
* surface the public share link on owner list detail ([#11](https://github.com/yaad-index/yaadegar/issues/11) F4) ([#71](https://github.com/yaad-index/yaadegar/issues/71)) ([8382763](https://github.com/yaad-index/yaadegar/commit/838276311316d254b352d19dd73a3bd5edb02a1f))


### Bug Fixes

* bound the login rate-limiter's memory ([#65](https://github.com/yaad-index/yaadegar/issues/65)) ([#66](https://github.com/yaad-index/yaadegar/issues/66)) ([49eef1a](https://github.com/yaad-index/yaadegar/commit/49eef1a7d68b79a3479d4e77f3e7005c378a2dea))
* check the DELETE response error in the item delete action ([#11](https://github.com/yaad-index/yaadegar/issues/11) F2) ([#69](https://github.com/yaad-index/yaadegar/issues/69)) ([bd56b48](https://github.com/yaad-index/yaadegar/commit/bd56b48aac89e3f89ce8f3ff0ae348c0868b655a))
* keep release-please pre-1.0 (bump-minor-pre-major) ([#50](https://github.com/yaad-index/yaadegar/issues/50)) ([8bc997a](https://github.com/yaad-index/yaadegar/commit/8bc997a68c33f48c99dd604ed61f0bc221aa58c1))
* set release-please initial-version to 0.1.0 ([#51](https://github.com/yaad-index/yaadegar/issues/51)) ([00785b4](https://github.com/yaad-index/yaadegar/commit/00785b42ced0989fbcb81f783703bfe8798fb7e3))
