# Migrations

SQLite selection is an M0 spike and requires approval before a driver is added.
The future adapter must enable `PRAGMA foreign_keys=ON`, WAL mode, and a bounded
`busy_timeout` before applying migrations. This repository contains contracts,
not a selected driver or operational persistence.
