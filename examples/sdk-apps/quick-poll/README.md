# Quick Poll

A private, replaceable preference poll demonstrating:

- `kv.get()` for one current poll definition;
- viewer-keyed votes from the server-derived identity;
- `kv.set()` with optimistic versions to create or change a vote;
- `kv.delete()` to withdraw it;
- bounded prefix pagination and aggregation; and
- a custom live channel used only as a refresh hint.

This is not a secret or tamper-proof ballot: every allowed viewer runs
untrusted app JavaScript within the shared app capability boundary. Build from
the parent directory with `npm run build`, then run `tinker deploy .` here.
