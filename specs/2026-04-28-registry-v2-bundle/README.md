---
status: idea
---
# Registry+v2 Bundle Format

Define a next-generation bundle format (registry+v2) with key improvements over registry+v1:

- **Phases** — ordered grouping of resources with progression semantics (phase N must be healthy before phase N+1 is applied).
- **Templating** — support for parameterized bundles where values can be injected at render time.
- **Arbitrary resource kinds** — no hardcoded assumptions about which Kubernetes resource types a bundle can contain (no special-casing of CSVs, CRDs, etc.).
- **No notion of scope** — bundles are not inherently namespace-scoped or cluster-scoped; scope is determined by the resources themselves.
- **Progression probes** — bundles define probes (condition checks, field value assertions) that determine when each phase is healthy. See `ProgressionProbe`, `Assertion`, and `ObjectSelector` patterns from operator-controller's `ClusterObjectSet` API for inspiration.
- **Metadata** — a key design element of the format. During refinement, survey other software packaging formats (e.g. Debian, RPM, Helm, OCI, Flatpak, Snap, npm, Cargo, etc.) to understand common metadata patterns and differences, then decide how to structure bundle metadata from those findings.
