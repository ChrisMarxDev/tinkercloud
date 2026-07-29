# Distribution Skill Denial Charter

- An inspect or prepare request never publishes, reserves a namespace, creates
  a hosted release, changes a tap, or advances a registry tag.
- A placeholder name/origin, version drift, unsigned/incomplete release,
  unknown source commit, unowned namespace, unspecified channel, unavailable
  signing authority, or missing registry authorization blocks publication.
- A CLI candidate containing `tinyhost`, a lifecycle downloader, runtime
  dependency, unsupported platform, or divergent package-manager payload is
  rejected.
- A CLI candidate whose npm command, launcher, native artifacts, Homebrew
  formula, installer, or documentation disagree after the rename is rejected.
- An SDK candidate with npm/JSR/export version drift, a runtime/peer dependency,
  install hook, unexpected file, CDN import, API-range drift, or unpinned JSR
  publisher is rejected.
- Registry, signing, tap, release, provider, operator, deployer, and viewer
  credentials never appear in commands, artifacts, chat, output, or evidence.
- A partial publication never triggers an overwrite, re-sign, hidden rebuild,
  silent `latest` change, or success report.
