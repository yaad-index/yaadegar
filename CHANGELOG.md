# Changelog

## [0.14.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.13.0...yaadegar-v0.14.0) (2026-08-24)


### Features

* list-summary item previews for the dashboard card cluster ([5f54a9c](https://github.com/yaad-index/yaadegar/commit/5f54a9cde0320d21606d3e606cf457265916a60c))
* **web:** complete the landing page — mockup, self-hosting, CTA, footer ([#236](https://github.com/yaad-index/yaadegar/issues/236)) ([#283](https://github.com/yaad-index/yaadegar/issues/283)) ([36af6ec](https://github.com/yaad-index/yaadegar/commit/36af6ecebefd85cdce1dfbef622f280b40ee65a1))
* **web:** instance-supplied landing page via YAADEGAR_ROOT_PAGE ([#256](https://github.com/yaad-index/yaadegar/issues/256)) ([#291](https://github.com/yaad-index/yaadegar/issues/291)) ([24dc437](https://github.com/yaad-index/yaadegar/commit/24dc437a872797e5122db8f3adc155f6a5110bcf))

## [0.13.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.12.0...yaadegar-v0.13.0) (2026-08-21)


### Features

* **build:** report the nearest release tag on a source build ([#259](https://github.com/yaad-index/yaadegar/issues/259)) ([6ac1691](https://github.com/yaad-index/yaadegar/commit/6ac169112bbbd1707048939335ee034755e91a84))
* publish the web image as a matched pair ([#268](https://github.com/yaad-index/yaadegar/issues/268)) ([#271](https://github.com/yaad-index/yaadegar/issues/271)) ([8413408](https://github.com/yaad-index/yaadegar/commit/8413408cf0a0678c934ebbf704067a2053c50cb8))
* self-hosting compose that CI proves serves a site ([#270](https://github.com/yaad-index/yaadegar/issues/270)) ([#274](https://github.com/yaad-index/yaadegar/issues/274)) ([5fd3aa8](https://github.com/yaad-index/yaadegar/commit/5fd3aa866c2895e693303eb8b446cfa05a8bbc48))
* version endpoints so a mismatched image pair says so ([#269](https://github.com/yaad-index/yaadegar/issues/269)) ([#273](https://github.com/yaad-index/yaadegar/issues/273)) ([377e63e](https://github.com/yaad-index/yaadegar/commit/377e63ee689413f561b1e2f76747dce3b352d792))


### Bug Fixes

* **web:** call the app's own login action "Log in", not "Sign in" ([#204](https://github.com/yaad-index/yaadegar/issues/204)) ([224418c](https://github.com/yaad-index/yaadegar/commit/224418c1f8a6de60a0ce754eb896be9aa3b8fd3c))
* **web:** read "Registration" when self-registration is off, not "Create an account" ([eff05e9](https://github.com/yaad-index/yaadegar/commit/eff05e9cec8db05839e3400a6559ad7ee379e065))
* **web:** show a not-enabled state on /register when self-registration is disabled ([#253](https://github.com/yaad-index/yaadegar/issues/253)) ([ab1505b](https://github.com/yaad-index/yaadegar/commit/ab1505b9a4b2c2739c1ed207dc4a12c128051478))
* **web:** word the anti-bot notice for both reserve and pledge ([#251](https://github.com/yaad-index/yaadegar/issues/251)) ([214c2d3](https://github.com/yaad-index/yaadegar/commit/214c2d3700859f9bc503c6f229a43eb4c08739bc))

## [0.12.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.11.0...yaadegar-v0.12.0) (2026-08-20)


### Features

* **web:** add the how-it-works, built-differently, and pull-quote landing sections ([#236](https://github.com/yaad-index/yaadegar/issues/236)) ([#254](https://github.com/yaad-index/yaadegar/issues/254)) ([cf1864e](https://github.com/yaad-index/yaadegar/commit/cf1864e0dd6f727486a3432e3be10375cfe87db4))
* **web:** add the mobile shell — collapsed header + nav drawer ([#232](https://github.com/yaad-index/yaadegar/issues/232)) ([978d260](https://github.com/yaad-index/yaadegar/commit/978d2609a17bbfe62f3d627dda288fd63afeeb97))
* **web:** add the public marketing landing page and move the dashboard to /lists ([#236](https://github.com/yaad-index/yaadegar/issues/236)) ([#252](https://github.com/yaad-index/yaadegar/issues/252)) ([30d3d40](https://github.com/yaad-index/yaadegar/commit/30d3d4008c99a5b15990f261ebbfdc070c9c36a1))
* **web:** credit the designer beneath the landing pull quote ([#236](https://github.com/yaad-index/yaadegar/issues/236)) ([#257](https://github.com/yaad-index/yaadegar/issues/257)) ([1c571fe](https://github.com/yaad-index/yaadegar/commit/1c571fe2aef69b540858df9d6a011ce03e03db3c))
* **web:** migrate the account area — settings and reserved-by-me ([#237](https://github.com/yaad-index/yaadegar/issues/237)) ([e7846d5](https://github.com/yaad-index/yaadegar/commit/e7846d51c5907c2c11d03e73dbb227ec83b2db8b))
* **web:** migrate the list Settings tab to the design system ([#230](https://github.com/yaad-index/yaadegar/issues/230)) ([95ceebb](https://github.com/yaad-index/yaadegar/commit/95ceebbeda5d49f8f2527b2b03bffd0b4f2929cb)), closes [#222](https://github.com/yaad-index/yaadegar/issues/222)
* **web:** migrate the public guest page to the design system ([#245](https://github.com/yaad-index/yaadegar/issues/245)) ([34632c7](https://github.com/yaad-index/yaadegar/commit/34632c7df101f39f23dba8281e298d691442ff6e))


### Bug Fixes

* report a source build's version from its embedded VCS commit, not "dev" ([#248](https://github.com/yaad-index/yaadegar/issues/248)) ([5a07b44](https://github.com/yaad-index/yaadegar/commit/5a07b4494c7376dced609c520dbcec2d23fb2981))
* **web:** clear the session cookie with protocol-derived secure so logout works over plain http ([#240](https://github.com/yaad-index/yaadegar/issues/240)) ([03910a1](https://github.com/yaad-index/yaadegar/commit/03910a1c68c6e60070a697aa98a9fefbbc6e825a))
* **web:** exempt undo and read actions on the guest page from the anti-bot check ([#249](https://github.com/yaad-index/yaadegar/issues/249)) ([#250](https://github.com/yaad-index/yaadegar/issues/250)) ([a7ff39a](https://github.com/yaad-index/yaadegar/commit/a7ff39a3c5011fb6f03316825bc9a3984c7f258e))
* **web:** give each cookie a single set/clear owner so their attributes can't drift ([#244](https://github.com/yaad-index/yaadegar/issues/244)) ([052304f](https://github.com/yaad-index/yaadegar/commit/052304fc85e5c6c1ff48d6a71fbabff3c0703fe3))
* **web:** key display headings by role, not size ([#226](https://github.com/yaad-index/yaadegar/issues/226)) ([8aefaff](https://github.com/yaad-index/yaadegar/commit/8aefaff6e54135f858d9a52a43ac953f0594bb9a))
* **web:** make the anti-bot widget's silent form-block visible on the guest page ([#247](https://github.com/yaad-index/yaadegar/issues/247)) ([0aa5a65](https://github.com/yaad-index/yaadegar/commit/0aa5a65f2fdf2e448a602db8a83c491339c90f39))
* **web:** pin the desktop wordmark at 24px, responsive ([#233](https://github.com/yaad-index/yaadegar/issues/233)) ([41a5e2f](https://github.com/yaad-index/yaadegar/commit/41a5e2fe9e93b3fca7c523de724511fbd31353cf))
* **web:** show an explicit note instead of a blank CNAME value when no target is configured ([#242](https://github.com/yaad-index/yaadegar/issues/242)) ([90e7298](https://github.com/yaad-index/yaadegar/commit/90e7298579ba682a888a1dc0dca3228a15eb2a4c))

## [0.11.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.10.0...yaadegar-v0.11.0) (2026-08-18)


### Features

* **web:** convert the signed-in area shell to the redesigned top bar ([#206](https://github.com/yaad-index/yaadegar/issues/206)) ([3ff2c96](https://github.com/yaad-index/yaadegar/commit/3ff2c968c939a5edcb4fdbb3a32cef55e6cf3c55))
* **web:** rebuild the auth screens on the design foundations ([#202](https://github.com/yaad-index/yaadegar/issues/202)) ([af2acb5](https://github.com/yaad-index/yaadegar/commit/af2acb5290479be088bca7c24b41bc10d4361b82))
* **web:** rebuild the dashboard on the design foundations ([#208](https://github.com/yaad-index/yaadegar/issues/208)) ([2c50e91](https://github.com/yaad-index/yaadegar/commit/2c50e91e9f8dc4f22284db99d2d29db6887f37cb))
* **web:** rebuild the owner list view (List tab) on the foundations ([#210](https://github.com/yaad-index/yaadegar/issues/210)) ([1727e18](https://github.com/yaad-index/yaadegar/commit/1727e1881480ef6e9d9aa74c4280573cd92c792d))
* **web:** self-host Fraunces and Inter, land the display type ([#218](https://github.com/yaad-index/yaadegar/issues/218)) ([#219](https://github.com/yaad-index/yaadegar/issues/219)) ([4f16c97](https://github.com/yaad-index/yaadegar/commit/4f16c972b97e3ec5700e6b23d026afbe42b9e24a))
* **web:** shared design foundations — tokens + components ([#200](https://github.com/yaad-index/yaadegar/issues/200)) ([3b87cd4](https://github.com/yaad-index/yaadegar/commit/3b87cd4ff0deaa64f76673eda48edd0ead7d2a82))


### Bug Fixes

* stamp the version into the released image at link time ([#191](https://github.com/yaad-index/yaadegar/issues/191)) ([6e2895e](https://github.com/yaad-index/yaadegar/commit/6e2895e2fb47db61795d03282b923abbb5201d5f))
* **web:** correct --accent-green-tint to the card icon chip colour ([#220](https://github.com/yaad-index/yaadegar/issues/220)) ([#221](https://github.com/yaad-index/yaadegar/issues/221)) ([9005bd3](https://github.com/yaad-index/yaadegar/commit/9005bd3b8ebe71f490de130372b163b9d48d5557))
* **web:** wrap the signed-in top bar on narrow viewports ([#212](https://github.com/yaad-index/yaadegar/issues/212)) ([0718e99](https://github.com/yaad-index/yaadegar/commit/0718e99315aac419be3098b430d1dab30ff5e13c))

## [0.10.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.9.0...yaadegar-v0.10.0) (2026-08-10)


### Features

* **profile:** self-serve display-name edit ([#185](https://github.com/yaad-index/yaadegar/issues/185)) ([#186](https://github.com/yaad-index/yaadegar/issues/186)) ([27420ee](https://github.com/yaad-index/yaadegar/commit/27420eedcb7cc99c21298d755518d95214cb2cdc))

## [0.9.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.8.0...yaadegar-v0.9.0) (2026-08-05)


### Features

* **captcha:** Altcha proof-of-work provider on low-trust reserve, cut 2 ([#45](https://github.com/yaad-index/yaadegar/issues/45)) ([#181](https://github.com/yaad-index/yaadegar/issues/181)) ([1aa992c](https://github.com/yaad-index/yaadegar/commit/1aa992c3b64060a73d6474fc734ecbdc04f1f5a6))
* **captcha:** anti-bot CAPTCHA on low-trust reserve, cut 1 ([#45](https://github.com/yaad-index/yaadegar/issues/45)) ([#180](https://github.com/yaad-index/yaadegar/issues/180)) ([46fc9aa](https://github.com/yaad-index/yaadegar/commit/46fc9aa01785745abd762efff3ad71095ef2db00))
* **captcha:** single-use Altcha solutions to defend against replay ([#182](https://github.com/yaad-index/yaadegar/issues/182)) ([#183](https://github.com/yaad-index/yaadegar/issues/183)) ([6f4a7a1](https://github.com/yaad-index/yaadegar/commit/6f4a7a1bf71b4df0a543979b1511808a668fbf7a))
* **cobuy:** thank-you notes for co-buy contributors ([#113](https://github.com/yaad-index/yaadegar/issues/113)) ([#184](https://github.com/yaad-index/yaadegar/issues/184)) ([dfe36b7](https://github.com/yaad-index/yaadegar/commit/dfe36b7e137ddd7d559cee32efc9d469e0f2a810))


### Bug Fixes

* **api:** move reset-token persist off the request path ([#159](https://github.com/yaad-index/yaadegar/issues/159)) ([#176](https://github.com/yaad-index/yaadegar/issues/176)) ([fe427de](https://github.com/yaad-index/yaadegar/commit/fe427de547352cfb4b2c7a2b690ae4bd8b3133c6))
* **storage:** tenant-scope the AddOwner insert ([#60](https://github.com/yaad-index/yaadegar/issues/60)) ([#178](https://github.com/yaad-index/yaadegar/issues/178)) ([4b9e0e3](https://github.com/yaad-index/yaadegar/commit/4b9e0e30c3ab0b08def881a218e14cb6e265ae88))

## [0.8.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.7.0...yaadegar-v0.8.0) (2026-08-04)


### Features

* **api:** expose account_required on the public list ([#170](https://github.com/yaad-index/yaadegar/issues/170)) ([#174](https://github.com/yaad-index/yaadegar/issues/174)) ([c4b4b09](https://github.com/yaad-index/yaadegar/commit/c4b4b09de08d93ca7d289b24e90e7dd149680336))
* **api:** resend verification on re-register of a pending email ([#162](https://github.com/yaad-index/yaadegar/issues/162)) ([#171](https://github.com/yaad-index/yaadegar/issues/171)) ([1cff95d](https://github.com/yaad-index/yaadegar/commit/1cff95d0f3b64079ed163c156517a6f7e52dcc8a))
* **auth:** email+password self-registration + owner-role gate (ADR-0012 cut 1a) ([#161](https://github.com/yaad-index/yaadegar/issues/161)) ([485144d](https://github.com/yaad-index/yaadegar/commit/485144dd3d162b91c8cee5624bf285ec6947fa7f))
* **auth:** OAuth self-register + no-password reconciliation (ADR-0012 cut 2) ([#167](https://github.com/yaad-index/yaadegar/issues/167)) ([1a1a243](https://github.com/yaad-index/yaadegar/commit/1a1a243811cf5d35056a2811355eec78890cf78b))
* **auth:** unified invite onboarding + establish-password reconciliation (ADR-0012 cut 1b) ([#165](https://github.com/yaad-index/yaadegar/issues/165)) ([5ffe4e6](https://github.com/yaad-index/yaadegar/commit/5ffe4e6e5d7cded7ab0ec015ec9915a59186fb77))
* **reserve:** registered-tier authenticated reserve + reserver dashboard (ADR-0012 cut 3a) ([#168](https://github.com/yaad-index/yaadegar/issues/168)) ([348a9fa](https://github.com/yaad-index/yaadegar/commit/348a9faa7fa13ce1e8e712bbd950a0e526bbbf29))
* **web:** reserve a registered-tier list from the browser ([#170](https://github.com/yaad-index/yaadegar/issues/170)) ([#175](https://github.com/yaad-index/yaadegar/issues/175)) ([5ad3b37](https://github.com/yaad-index/yaadegar/commit/5ad3b37702c2dcf2c1dad518bfc490a464bf706d))
* **web:** reserver dashboard + giver landing + registered-tier option/warning (ADR-0012 cut 3b) ([#169](https://github.com/yaad-index/yaadegar/issues/169)) ([02cab31](https://github.com/yaad-index/yaadegar/commit/02cab31b9e335d8eb77a471bef84e13ba930b42d)), closes [#20](https://github.com/yaad-index/yaadegar/issues/20)


### Bug Fixes

* **auth:** issue JWT with the account's real tenant role ([#163](https://github.com/yaad-index/yaadegar/issues/163)) ([#173](https://github.com/yaad-index/yaadegar/issues/173)) ([cc95acb](https://github.com/yaad-index/yaadegar/commit/cc95acb689466e6f027252bb4005a4f4cdc71cb9))
* **storage:** make password-reset/invite confirm transactional ([#166](https://github.com/yaad-index/yaadegar/issues/166)) ([#172](https://github.com/yaad-index/yaadegar/issues/172)) ([941507d](https://github.com/yaad-index/yaadegar/commit/941507dc10d83a3a692f1d9e472421b79dd4f03b))

## [0.7.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.6.0...yaadegar-v0.7.0) (2026-08-03)


### Features

* **auth:** authenticated change-password endpoint + Settings form (ADR-0011 cut 2) ([2110fb9](https://github.com/yaad-index/yaadegar/commit/2110fb9ca20a55d28f372df2cf3adb5f920496b9))
* **auth:** credential-version session invalidation + password funnel (ADR-0011 cut 1) ([5b600c8](https://github.com/yaad-index/yaadegar/commit/5b600c82895e98ad6df776011e4858117d5acbe3))
* **auth:** enumeration-safe forgot-password reset (ADR-0011 cut 3) ([#155](https://github.com/yaad-index/yaadegar/issues/155)) ([6e33448](https://github.com/yaad-index/yaadegar/commit/6e33448b70ed72aeae4f414ab4ed5c94e634c685)), closes [#142](https://github.com/yaad-index/yaadegar/issues/142) [#148](https://github.com/yaad-index/yaadegar/issues/148)
* **cli:** add set-password command for owner/admin password recovery ([#141](https://github.com/yaad-index/yaadegar/issues/141)) ([fa01752](https://github.com/yaad-index/yaadegar/commit/fa01752cd58829093debf97972d659bb3c69dcfb))
* **web:** list-level description with sanitized light-markdown ([#143](https://github.com/yaad-index/yaadegar/issues/143)) ([38f4122](https://github.com/yaad-index/yaadegar/commit/38f4122ab4f9a5cf9b43ec10440943e569329be7))
* **web:** make item price editable in add + edit forms ([#140](https://github.com/yaad-index/yaadegar/issues/140)) ([66d68e9](https://github.com/yaad-index/yaadegar/commit/66d68e92b0fcfa8e212520b246fb052c99654d67))
* **web:** split list detail into List and Settings tabs ([#128](https://github.com/yaad-index/yaadegar/issues/128)) ([#136](https://github.com/yaad-index/yaadegar/issues/136)) ([7085b18](https://github.com/yaad-index/yaadegar/commit/7085b18202a72665b1d5bb8d6fcf4c75e44543c6))
* **web:** transparent /api/v1 passthrough for external clients ([#145](https://github.com/yaad-index/yaadegar/issues/145)) ([ebd62d4](https://github.com/yaad-index/yaadegar/commit/ebd62d497499c94db4492b6653b5aafc7d56b504))


### Bug Fixes

* surface backend reason on failed reserve/chip-in; flag email-required lists ([#144](https://github.com/yaad-index/yaadegar/issues/144)) ([59fe54e](https://github.com/yaad-index/yaadegar/commit/59fe54e47af1c3c48193ba888c12294655084517))
* **web:** keep scraped prefill + pasted url on Fetch details (add-item form reset) ([#139](https://github.com/yaad-index/yaadegar/issues/139)) ([5dcaefb](https://github.com/yaad-index/yaadegar/commit/5dcaefb1dea9fa92495c123a2683b59d61dcabbb))
* **web:** make list-detail tab clicks switch the panel ([#128](https://github.com/yaad-index/yaadegar/issues/128)) ([#138](https://github.com/yaad-index/yaadegar/issues/138)) ([76f6f30](https://github.com/yaad-index/yaadegar/commit/76f6f30bf4260eadec7250c4b94a985831eb36d3))

## [0.6.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.5.0...yaadegar-v0.6.0) (2026-08-01)


### Features

* **admin:** admin as a per-user capability (ADR-0010) ([#134](https://github.com/yaad-index/yaadegar/issues/134)) ([82d995f](https://github.com/yaad-index/yaadegar/commit/82d995f2824a847ad3cdad0b3080cf6f165c849e))
* **admin:** greenfield admin frontend — login + user management (ADR-0009 Cut 1b) ([#131](https://github.com/yaad-index/yaadegar/issues/131)) ([62b1b25](https://github.com/yaad-index/yaadegar/commit/62b1b2519528b3aa099c92e7e61bdf64b18a6568))
* **admin:** show name and an admin badge in the users list ([#135](https://github.com/yaad-index/yaadegar/issues/135)) ([7351bf7](https://github.com/yaad-index/yaadegar/commit/7351bf7669f505033f21202734d6ae971b5cdb46))
* **admin:** user-management backend — roles, ban, /admin endpoints (ADR-0009 Cut 1a) ([#130](https://github.com/yaad-index/yaadegar/issues/130)) ([77bfd6f](https://github.com/yaad-index/yaadegar/commit/77bfd6fbb65e846b4e534f5a95765b27fb954f86))
* **api:** list catalog export — JSON + CSV ([#26](https://github.com/yaad-index/yaadegar/issues/26) Cut 1) ([#116](https://github.com/yaad-index/yaadegar/issues/116)) ([2ffe32f](https://github.com/yaad-index/yaadegar/commit/2ffe32f1384e368e4679d764d665a652b36d6ae1))
* **api:** list catalog import — JSON + CSV ([#26](https://github.com/yaad-index/yaadegar/issues/26) Cut 2) ([#117](https://github.com/yaad-index/yaadegar/issues/117)) ([82ef6d0](https://github.com/yaad-index/yaadegar/commit/82ef6d017fbb87ac7522256934eda7fe0dd668f5))
* **auth:** OAuth login frontend — passthrough + methods + Google button ([#21](https://github.com/yaad-index/yaadegar/issues/21)) ([#120](https://github.com/yaad-index/yaadegar/issues/120)) ([4cbcd5c](https://github.com/yaad-index/yaadegar/commit/4cbcd5c49d095ef756ec66d27e5d15368e43a725))
* **auth:** owner login via Google OAuth/OIDC — backend ([#21](https://github.com/yaad-index/yaadegar/issues/21)) ([#119](https://github.com/yaad-index/yaadegar/issues/119)) ([61051e7](https://github.com/yaad-index/yaadegar/commit/61051e76f76de0e00a319c6f38c2efabd8010206))
* **auth:** owner-settings Google-login toggle — OAuth Cut 2 ([#21](https://github.com/yaad-index/yaadegar/issues/21)) ([#121](https://github.com/yaad-index/yaadegar/issues/121)) ([9849050](https://github.com/yaad-index/yaadegar/commit/9849050e5f667d2d9e17c928c2f7390f163e53b6))
* **reservations:** owner thank-you notes for reservers ([#22](https://github.com/yaad-index/yaadegar/issues/22)) ([#114](https://github.com/yaad-index/yaadegar/issues/114)) ([4187a38](https://github.com/yaad-index/yaadegar/commit/4187a38c26c3a3915a3c1192a2845d2b971662dc))
* **web:** owner Settings UI for custom domains ([#122](https://github.com/yaad-index/yaadegar/issues/122)) ([#123](https://github.com/yaad-index/yaadegar/issues/123)) ([51046ff](https://github.com/yaad-index/yaadegar/commit/51046ff34ffee469df9a4b81634ae1a805da2a8d))
* **web:** per-list reserver-tier control ([#126](https://github.com/yaad-index/yaadegar/issues/126)) ([#127](https://github.com/yaad-index/yaadegar/issues/127)) ([f503015](https://github.com/yaad-index/yaadegar/commit/f503015ea5432249b85c51f7f4f388b5c0a136ca))


### Bug Fixes

* **auth:** render OAuth callback failures as a login-page message, not raw JSON ([#124](https://github.com/yaad-index/yaadegar/issues/124)) ([#125](https://github.com/yaad-index/yaadegar/issues/125)) ([d19f566](https://github.com/yaad-index/yaadegar/commit/d19f5666eee974b5a1774124ce83f72149739d3d))

## [0.5.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.4.0...yaadegar-v0.5.0) (2026-07-30)


### Features

* **api:** three-state clear-semantics for nullable override fields ([#111](https://github.com/yaad-index/yaadegar/issues/111)) ([#112](https://github.com/yaad-index/yaadegar/issues/112)) ([ed7a8ed](https://github.com/yaad-index/yaadegar/commit/ed7a8edf0b1322cd0bbf11ff7e1ea16946c9f48f))
* **cobuy:** auto-expire stale proposed matches ([#101](https://github.com/yaad-index/yaadegar/issues/101)) ([#109](https://github.com/yaad-index/yaadegar/issues/109)) ([7e84f68](https://github.com/yaad-index/yaadegar/commit/7e84f6860ba53ebf84d6e21ec22831d832af39c9))
* **cobuy:** make reserve and co-buy mutually exclusive per item ([#93](https://github.com/yaad-index/yaadegar/issues/93)) ([#107](https://github.com/yaad-index/yaadegar/issues/107)) ([5d719a2](https://github.com/yaad-index/yaadegar/commit/5d719a257d5164e68e3c6b241ea5bfbaed1a3b8b))
* **cobuy:** owner opt-out of group-buying ([#100](https://github.com/yaad-index/yaadegar/issues/100)) ([#110](https://github.com/yaad-index/yaadegar/issues/110)) ([67709a2](https://github.com/yaad-index/yaadegar/commit/67709a21e03e3e99c53b5fa32128b7430457cc87))
* cross-device co-buy — scoped match-action token + match-read endpoint ([#96](https://github.com/yaad-index/yaadegar/issues/96)) ([#103](https://github.com/yaad-index/yaadegar/issues/103)) ([31be210](https://github.com/yaad-index/yaadegar/commit/31be210d9d7d46be237e71fac67191adf4003cc6))
* cross-device co-buy handshake — /cobuy URL-token wiring ([#96](https://github.com/yaad-index/yaadegar/issues/96) Cut 2) ([#105](https://github.com/yaad-index/yaadegar/issues/105)) ([a07c147](https://github.com/yaad-index/yaadegar/commit/a07c147228176f661663ed07f87966c9c50b1c7d))

## [0.4.0](https://github.com/yaad-index/yaadegar/compare/yaadegar-v0.3.0...yaadegar-v0.4.0) (2026-07-30)


### Features

* co-buying confirm/decline handshake page ([#92](https://github.com/yaad-index/yaadegar/issues/92) Cut 2) ([#97](https://github.com/yaad-index/yaadegar/issues/97)) ([fea062c](https://github.com/yaad-index/yaadegar/commit/fea062cfd69a0839925ae43ff27c619c3cb68b47))
* co-buying pledge, track, and withdraw on the giver page ([#92](https://github.com/yaad-index/yaadegar/issues/92) Cut 1) ([#95](https://github.com/yaad-index/yaadegar/issues/95)) ([895bb6e](https://github.com/yaad-index/yaadegar/commit/895bb6e92c157f5c18995483ad0b996e97e4d0b6))
* giver-facing /confirm page for email_confirmed reservations ([#82](https://github.com/yaad-index/yaadegar/issues/82)) ([#87](https://github.com/yaad-index/yaadegar/issues/87)) ([76fc6c0](https://github.com/yaad-index/yaadegar/commit/76fc6c02613dc6b61242a8f26550a1892df95dfd))
* per-list email_confirmed confirm-window override ([#81](https://github.com/yaad-index/yaadegar/issues/81)) ([#89](https://github.com/yaad-index/yaadegar/issues/89)) ([1be83c6](https://github.com/yaad-index/yaadegar/commit/1be83c69cb6a9aae59c7558a4107ade2f789ced8))


### Bug Fixes

* roll back an email_confirmed hold when the confirm email can't send ([#86](https://github.com/yaad-index/yaadegar/issues/86)) ([#91](https://github.com/yaad-index/yaadegar/issues/91)) ([b3ead27](https://github.com/yaad-index/yaadegar/commit/b3ead272dd6d47c7dcc6dec82caf460c34e2260a))

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
