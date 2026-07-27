# M5 TinyHost port-bind denial charter

TinyHost changes the host network surface only by opening its HTTP and HTTPS
gateway listeners. It does not claim ownership of the operator's firewall, SSH
service, or pre-existing processes.

Before the port hardening slice is complete, executable evidence must prove:

- production configuration rejects HTTP outside port 80 and HTTPS outside port
  443;
- the packaged and generated systemd units default-deny socket binds and allow
  only TCP 80 and TCP 443;
- removing or broadening any bind-policy directive makes local unit validation
  fail before `systemctl` runs;
- a live inventory with healthy TinyHost listeners on 80/443 plus a
  TinyHost-owned listener on any other non-loopback port fails;
- a pre-existing listener owned by another process is not attributed to
  TinyHost; and
- firewall and SSH configuration are not mutated by installation.
