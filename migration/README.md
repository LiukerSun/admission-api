# Migration Notes

Database changes are executed by `golang-migrate` in numeric order.

This project uses a single compact baseline migration:

- `001_init_schema.up.sql` creates the full application schema, indexes, and seed data.
- `001_init_schema.down.sql` drops all tables in reverse dependency order.

The baseline is laid out by FK dependency in eight sections — accounts/payments,
dictionaries, major catalog, universities, admissions, conversations,
recommendation metadata, seed data. Future changes should land as new
`002_*.up.sql` / `002_*.down.sql` files rather than editing the baseline once a
shared environment has applied it.

Use `schema/current.sql` as a readable snapshot of the current schema. Use
`migration/` for actual database setup and rollbacks.
