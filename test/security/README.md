# Security Test Charters

Future tests in this directory should hold reusable hostile corpora and
end-to-end denial proofs:

- host and path parser corpus;
- archive attack corpus;
- anonymous protected-surface matrix;
- cross-app isolation matrix;
- OTP enumeration/rate-limit cases;
- session and policy revocation timing;
- deployment interruption/failure injection;
- log/audit secret scanning;
- deterministic checked-in secret-pattern deny/allow scan and bounded parser
  fuzz gates (`scripts/ci-security-gates.sh`), including gitignore visibility
  assertions for local operator secrets and the committed release public key;
- socket inventory and origin bypass checks;
- signed update integrity, failed-health rollback, and root recovery.

The detailed matrix lives in
[`docs/security/test-matrix.md`](../../docs/security/test-matrix.md).
