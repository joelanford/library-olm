---
status: idea
---
# Public Catalog Builder

Move the catalog builder test utility (`catalog/fbc/internal/testing/catalogfs`) to a public API so it can be used by consumers of library-olm and by tests in other packages (e.g., `catalog/v1` fingerprint tests) without hitting internal package import restrictions.
