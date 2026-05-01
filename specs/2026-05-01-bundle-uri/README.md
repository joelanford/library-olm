---
status: idea
---
# Bundle URI

Add `URI() string` to the `bundlev1.Bundle` interface so that bundles returned from catalog queries carry their content location. This bridges the catalog discovery layer to existing fetch/unpack tools (e.g. `image/bundle/` handlers) without the catalog needing to be opinionated about how content is retrieved.

## Key Design Points

- **`Bundle` interface expands:** `URI() string` joins `Name()` and `VersionRelease()`. Every `Bundle` implementation must provide a URI.
- **`NameVersionRelease` is unchanged:** It stops implementing `Bundle` — it remains a pure identity value type. No modifications needed.
- **FBC returns a richer type:** The FBC query layer returns a concrete bundle type that composes identity fields (name, version, release) with a URI. This type implements the expanded `Bundle` interface.
- **URI includes scheme:** FBC stores the URI with a scheme prefix (e.g. `docker://quay.io/foo/bar:v1.0`) in the normalized `bundles` table. The scheme enables callers to dispatch to the appropriate fetcher without parsing conventions.
- **Scope is content URI only:** Related images, properties, and other metadata are separate concerns for future work.
