---
status: done
pr: https://github.com/joelanford/library-olm/pull/3
---
# FBC skipRange Migration to Predecessor Ranges

Migrate FBC's skipRange handling to use the predecessor range mechanism (from `2026-05-06-semver-successor-constraints`) instead of expanding skipRange into explicit edges at ingest time. Previously, `writeSkipRangeSuccessors` iterated all bundles in a channel whose version matched the range and inserted an explicit successor edge for each. With predecessor ranges, the skipRange string is stored directly via `AddPredecessorRange` and evaluated dynamically at query time.

Depends on: `2026-05-06-semver-successor-constraints`

Deliverables:
- FBC handler stores skipRange as a predecessor range via `AddPredecessorRange` instead of expanding into explicit edges
- Remove the `writeSkipRangeSuccessors` expansion logic
- Verify existing FBC catalogs produce equivalent successor results
- Simplification of FBC handler code
