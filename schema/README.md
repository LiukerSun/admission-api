# Database Schema

This directory contains the current database shape as a readable snapshot.

- `current.sql` is a consolidated schema for the application after removing the old `gaokao` domain.
- Runtime database changes are still managed by `migration/` through `golang-migrate`.
- Do not apply `current.sql` to an existing database as a migration. Use it for review, onboarding, and fresh schema reference.

