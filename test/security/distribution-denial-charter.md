# Distribution Preparation Denial Charter

- A version mismatch across server, CLI, SDK, npm, JSR, or generated formula
  fails before a candidate package is written.
- Missing platforms, unsigned bytes, checksum drift, manifest drift, unknown
  files, symlinks, path traversal, lifecycle scripts, runtime package
  dependencies, or an executable other than `tiny` fail preparation.
- Package preparation cannot run `npm publish`, `jsr publish`, `brew tap`,
  `brew push`, `gh release`, GitHub mutation, or a network upload.
- The npm launcher denies unsupported OS/architecture combinations and never
  downloads or executes a postinstall payload.
- Formula generation accepts only the four known workstation artifacts and
  never writes outside its explicit output directory.
- No preparation path reads a beta or production signing key except the already
  explicit signed-release build.
