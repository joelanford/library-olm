---
status: idea
---
# FBC skipRange Migration to Successor Constraints

Migrate FBC's skipRange handling to use the semver successor constraint mechanism (from `2026-05-06-semver-successor-constraints`) instead of expanding skipRange into explicit edges at ingest time. Currently, `writeSkipRangeSuccessors` iterates all bundles in a channel whose version matches the range and inserts an explicit successor edge for each. With the constraint mechanism, the skipRange string is stored directly via `AddPredecessorConstraint` and evaluated dynamically at query time.

Depends on: `2026-05-06-semver-successor-constraints`

Deliverables:
- FBC handler stores skipRange as a constraint via `AddPredecessorConstraint` instead of expanding into explicit edges
- Remove the `writeSkipRangeSuccessors` expansion logic
- Verify existing FBC catalogs produce equivalent successor results
- Simplification of FBC handler code
