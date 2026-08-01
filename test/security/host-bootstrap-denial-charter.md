# Workstation Host Bootstrap Denial Charter

- A target without exact `root@HOST` form, a target beginning with `-`, an SSH
  option, whitespace, control character, shell metacharacter, path, port
  suffix, or URI syntax is rejected before OpenSSH starts.
- Non-HTTPS, credential-bearing, query-bearing, fragment-bearing, loopback,
  link-local, private, malformed, or shell-bearing release origins are rejected
  before SSH starts.
- Host-key failure, unknown host, authentication failure, unavailable OpenSSH,
  interrupted stdin transport, or non-zero remote exit never reports success.
- `tinker host` cannot execute an arbitrary command, change SSH policy, edit
  `known_hosts`, or transfer deployer credentials.
- The bootstrap rejects non-root, non-Linux, non-amd64, missing verification
  tools, redirect, oversized response, checksum mismatch, metadata drift,
  signature failure, or service-unit validation failure before replacement.
