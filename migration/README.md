# Migration Notes

Database changes are executed by `golang-migrate` in numeric order.

This project currently uses a compact baseline migration:

- `001_init_schema.up.sql` creates the current application schema.
- `001_init_schema.down.sql` drops the application tables in dependency order.

Use `schema/current.sql` as a readable snapshot of the current schema. Use `migration/` for actual database setup and rollbacks.
