# Changelog

## [0.3.0](https://github.com/braydend/fleet/compare/v0.2.0...v0.3.0) (2026-06-17)


### Features

* **selfupdate:** add minio/selfupdate adapter for binary swap ([65c96fb](https://github.com/braydend/fleet/commit/65c96fb4f777dda56a67aeb9f8665e0d5da52bcc))
* **selfupdate:** add semver comparison helpers ([4a49c0b](https://github.com/braydend/fleet/commit/4a49c0b12f3ee0a120b73f195c4f6344d19cc565))
* **selfupdate:** add throttle state for check timing ([cca6d4e](https://github.com/braydend/fleet/commit/cca6d4e2bdc2888eb24c543e9d98d731e013b18b))
* **selfupdate:** extract fleet binary from release archive ([d65aaad](https://github.com/braydend/fleet/commit/d65aaad6cde1d801d578b0bba096366561148e05))
* **selfupdate:** in-place self-update from GitHub Releases ([#21](https://github.com/braydend/fleet/issues/21)) ([1f74380](https://github.com/braydend/fleet/commit/1f74380f3ee2c1a9b2d13137e7036c6f07a1c5ac))
* **selfupdate:** orchestrate download, verify, and binary swap ([3c71450](https://github.com/braydend/fleet/commit/3c714504a38980757632ded0eef519f9088c59d5))
* **selfupdate:** query GitHub latest release and compare versions ([32c5edc](https://github.com/braydend/fleet/commit/32c5edc2b8b980610a691947e3c8254700e16003))
* **selfupdate:** select platform asset and verify checksum ([a830381](https://github.com/braydend/fleet/commit/a83038171567c9bc404e3a915f80583a87a393a5))
* **selfupdate:** wire throttled update check and apply into main ([4e000d3](https://github.com/braydend/fleet/commit/4e000d3bc18137b5a717712932f59b159553958d))
* **ui:** add versionLabel helper for dashboard version ([2aa7868](https://github.com/braydend/fleet/commit/2aa78680444d0becedcdc3a3cd72671556e52cf8))
* **ui:** render update banner and confirm dialog ([ab10259](https://github.com/braydend/fleet/commit/ab10259be456c91130cbb7de4560c9e73c95b2df))
* **ui:** show running version in dashboard footer ([55830db](https://github.com/braydend/fleet/commit/55830dbf4492e0a72ed517d9b42cf6b9fa196ba8))
* **ui:** wire self-update check, confirm, and apply flow ([2f2af1f](https://github.com/braydend/fleet/commit/2f2af1f7e3fe5a25f3ce6ea7c5c256632c5ab2a1))


### Bug Fixes

* **selfupdate:** map swap-time permission errors to manual-install hint ([b8e26e3](https://github.com/braydend/fleet/commit/b8e26e396e0caed12b3631077364a4a8c6243980))

## [0.2.0](https://github.com/braydend/fleet/compare/v0.1.1...v0.2.0) (2026-06-17)


### Features

* **ui:** add gradient dashboard title ([c90ce2a](https://github.com/braydend/fleet/commit/c90ce2af533ca70c71ca41b9758fade17174174a))
* **ui:** add rounded project-box helper ([1bcb5ff](https://github.com/braydend/fleet/commit/1bcb5ffccb2c51112a558a4aaa4f14ede49b9862))
* **ui:** animate a spinner on working sessions ([5d531f7](https://github.com/braydend/fleet/commit/5d531f72936fbb6d48e6f690a8917ca2d2e29a73))
* **ui:** prettier neon TUI ([#13](https://github.com/braydend/fleet/issues/13)) ([af5ce95](https://github.com/braydend/fleet/commit/af5ce9579cb1adce6b50c75230e5c0c3d8e625e3))
* **ui:** restyle picker, form, cleanup, and confirm screens ([ed434f9](https://github.com/braydend/fleet/commit/ed434f964982189ae12aa1d4982389dbf2311ac9))
* **ui:** use emoji status icons across the dashboard ([98866fc](https://github.com/braydend/fleet/commit/98866fc8c956a7e1fb2b86c00e4f2c9bc6ecb409))
* **ui:** wrap dashboard project groups in rounded boxes ([e1003e0](https://github.com/braydend/fleet/commit/e1003e0917c0962137259b178117afdfbc3de85b))

## [0.1.1](https://github.com/braydend/fleet/compare/v0.1.0...v0.1.1) (2026-06-17)


### Bug Fixes

* **tmux:** restore paste with mouse mode on ([#14](https://github.com/braydend/fleet/issues/14)) ([54676ef](https://github.com/braydend/fleet/commit/54676efcbaa5f5836634a9f71e7c6462288e70b3))
* **tmux:** restore paste with mouse mode on ([#14](https://github.com/braydend/fleet/issues/14)) ([543e607](https://github.com/braydend/fleet/commit/543e6074073d1bdd63e0a2604697f419dafccf36))

## 0.1.0 (2026-06-16)


### Features

* **activity:** pure session activity classifier ([7bc8ff0](https://github.com/braydend/fleet/commit/7bc8ff0c25c29287ac790e5c15da7bcea2fdc664))
* add --version flag with build metadata ([7d188ee](https://github.com/braydend/fleet/commit/7d188ee6ec365582b13f38b2832784759e7360b7))
* **config:** interactive first-run setup prompts for scan_root ([0efe270](https://github.com/braydend/fleet/commit/0efe270a7ff5e22e69b66c73c11f8de6bce3a530))
* **config:** load/validate config with defaults ([82a68ac](https://github.com/braydend/fleet/commit/82a68ac933063e3446740e9245ace65fa20f50ae))
* fleet MVP — TUI for managing multiple Claude Code sessions ([2a10803](https://github.com/braydend/fleet/commit/2a10803b14451b931972a232fa6965cc4accd5b3))
* **forge:** gh pull-request adapter ([d26ecdf](https://github.com/braydend/fleet/commit/d26ecdf175e34c20442847cfccbce65432de49a5))
* **git:** CLI worktree/branch/status adapter ([ad0234d](https://github.com/braydend/fleet/commit/ad0234d5ffac14a82911d07ccc8935b90b162b46))
* **main:** attach via shared workspace with tab switching ([ae91bac](https://github.com/braydend/fleet/commit/ae91bacafd345510ed41a98dfca650fb6d7f9303))
* **meta:** read/write per-worktree meta.json ([860047f](https://github.com/braydend/fleet/commit/860047f750ecc5f345beb2d33732b0440d27dd58))
* **naming:** tmux name and worktree path encoding ([6caa380](https://github.com/braydend/fleet/commit/6caa380078e5074af3d38a6bdca94041998a853b))
* **naming:** workspace constant and window target helper ([4c2c2eb](https://github.com/braydend/fleet/commit/4c2c2ebeeee2966c775f01ba65d16eb421d29475))
* **projects:** scan root dir for git repos ([f8658ac](https://github.com/braydend/fleet/commit/f8658ac1c96fadc743c7039004b68097df562763))
* **refresher:** derive live sessions from disk+tmux+git ([39ee9ff](https://github.com/braydend/fleet/commit/39ee9ffae2fd1234aa36a7e93c52e81781b9d792))
* **refresher:** window mapping, activity classification, tab labels ([59b4f89](https://github.com/braydend/fleet/commit/59b4f896e950f1135fdfe632672b4b8bf8f011b1))
* **session:** activity, last-activity, and window-index fields ([4d73cba](https://github.com/braydend/fleet/commit/4d73cba3309fbc93b90348e8ee3ea2271d20cae8))
* **session:** Session type and Manager.Create ([374256e](https://github.com/braydend/fleet/commit/374256ea1fe9dd993c87104242fba4ec820fcac5))
* **session:** teardown, push/PR, and attach ([48d33a7](https://github.com/braydend/fleet/commit/48d33a7d3be7857e789eed42c15d6066f13cdc79))
* **session:** window-based lifecycle in the shared workspace ([a473d8c](https://github.com/braydend/fleet/commit/a473d8c00d1b77278c806e7e800ae526e8628fca))
* **tmux:** CLI session adapter ([c40e077](https://github.com/braydend/fleet/commit/c40e077125167ea93a8b2d0b2e3aa0d2b47172de))
* **tmux:** run fleet on a dedicated tmux server (socket "fleet") ([4ce4518](https://github.com/braydend/fleet/commit/4ce45181b86a90f6948450cc50bf2ceba2bdf6b6))
* **tmux:** show an in-session status bar with navigation help ([9ae62bf](https://github.com/braydend/fleet/commit/9ae62bf385008af4a7cb5b4de63d3c2524a91309))
* **tmux:** tab strip configuration and workspace attach/select ([03ef99e](https://github.com/braydend/fleet/commit/03ef99eb262168170ce2ff51a5baf0a2e058524a))
* **tmux:** workspace window lifecycle and queries ([90fdb17](https://github.com/braydend/fleet/commit/90fdb17762a7cc812009e7b1625e77bfa8166e7d))
* **ui:** attach, cleanup menu, and delete confirmation ([a378930](https://github.com/braydend/fleet/commit/a3789307fba4b4642138ce2030d1f7c899a43afb))
* **ui:** default new-session branch name to the session name ([344e50f](https://github.com/braydend/fleet/commit/344e50f33eb308d2307c43093d4c9589bd8a936e))
* **ui:** grouped dashboard with tab numbers, activity glyphs, legend ([d0f55c8](https://github.com/braydend/fleet/commit/d0f55c88132258ef20d46dc4b94e52467e4d8d3d))
* **ui:** model skeleton, dashboard view, refresh tick ([45fb9ee](https://github.com/braydend/fleet/commit/45fb9eec2c8fee3369e92c7f10ba0f9cf96623ce))
* **ui:** project picker and new-session form ([7faaac2](https://github.com/braydend/fleet/commit/7faaac27d6ff5ae4b10a4a03c89af44740d142f0))
* wire config, adapters, manager, refresher into the TUI ([ad507b7](https://github.com/braydend/fleet/commit/ad507b7d3564f3ca74f7ba919c0ba4a3d50ee48a))


### Bug Fixes

* **git:** exclude fleet's .fleet/ bookkeeping from worktree status ([c100818](https://github.com/braydend/fleet/commit/c100818b0ca61086f4a881d930219f993c7b512f))
* **refresher:** escape '#' in tab labels from user-supplied names ([61a966f](https://github.com/braydend/fleet/commit/61a966f90264b2bf68eea4fb04e1d33255481a24))
* **session:** restart an exited session when attaching ([3257d1e](https://github.com/braydend/fleet/commit/3257d1ec425067bd545fd5c999e53d7f2fbe4629))
* **tmux:** enable mouse mode so the wheel scrolls scrollback ([#2](https://github.com/braydend/fleet/issues/2)) ([609eb65](https://github.com/braydend/fleet/commit/609eb65ee26c03b869537922554e52f1cf5f0f8a))
* **tmux:** enable mouse mode so the wheel scrolls scrollback ([#2](https://github.com/braydend/fleet/issues/2)) ([3a05eb6](https://github.com/braydend/fleet/commit/3a05eb6c840e7606770e7ea1b7c4b207d621ccb4))
* **tmux:** isolate fleet onto its own tmux server (fixes [#5](https://github.com/braydend/fleet/issues/5)) ([3f53360](https://github.com/braydend/fleet/commit/3f533605423303d37a505f313add4b50722c8f79))
* **tmux:** isolate tests onto a private socket so they can't kill live sessions ([3035b54](https://github.com/braydend/fleet/commit/3035b5412be0609c3521ae4bd96683a6a0d2942c))
* **ui,forge:** show worktree path + created-at on dashboard; surface gh errors ([073142b](https://github.com/braydend/fleet/commit/073142bc9a11c3a440a0d7c0bff184310e2e427c))
* **ui:** don't let the periodic refresh reset the active screen ([55e9fa0](https://github.com/braydend/fleet/commit/55e9fa0d8326e4be832843af54ca53580cff7b36))
* **ui:** track full session name as branch default while typing ([26aedc1](https://github.com/braydend/fleet/commit/26aedc14982ed2fb29268799325a9fcf58fa8311))


### Miscellaneous Chores

* enable automated releases ([22ca95d](https://github.com/braydend/fleet/commit/22ca95d56d077b03e6907a3cab4aff0de92efde4))
