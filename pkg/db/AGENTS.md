# Database package guidance

## Tests

- Prefer exercising database behavior through the same sqlc queries and service helpers used by application code.
- Do not use ad hoc direct SQL in tests for normal setup, mutation, or assertions when an app sqlc query exists.
- If a test needs database access that production code intentionally does not expose, add a narrowly scoped sqlc test/support query instead of embedding raw SQL in the test.
- Direct SQL in tests is acceptable only for true schema/migration assertions or unavoidable low-level diagnostics; keep it small and documented inline.
- When schema changes remove or replace a column/table, update tests to use the replacement query path rather than preserving legacy direct SQL against the old shape.
