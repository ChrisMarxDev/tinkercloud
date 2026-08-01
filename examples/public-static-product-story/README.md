# Northstar public static product story

This is a first-party, capability-free public-static example. Its v2 manifest
uses explicit `access.mode: public`, deliberately opts into indexing, and has no
`features` or `capabilities` block.

It is not a starter template: public activation still requires the current
operator gate and the deployer's explicit `tinker deploy --confirm-public .`
acknowledgement. The default starter and SDK gallery remain private.
