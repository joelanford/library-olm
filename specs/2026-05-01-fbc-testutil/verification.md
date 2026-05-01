# Verification

## Implementation Correctness

- [ ] Builder produces `fstest.MapFS` with one file per blob under `<package>/` directories
- [ ] Each file contains a single JSON blob parseable by `declcfg.WalkMetasFS`
- [ ] Default bundle name is `<package>.v<version>`
- [ ] Default bundle image is `quay.io/<package>/bundle:v<version>`
- [ ] Default bundle has `olm.package` property with correct `packageName` and `version`
- [ ] `WithName` overrides the bundle name in the JSON blob
- [ ] `WithImage` overrides the image in the JSON blob
- [ ] `WithRelease` adds the release field to the `olm.package` property
- [ ] `Replaces` resolves version to `<package>.v<version>` in the channel entry
- [ ] `Skips` resolves each version to `<package>.v<version>` in the channel entry
- [ ] `SkipRange` passes the range string through without resolution
- [ ] Multiple packages can be built in a single builder call
- [ ] `Build()` panics on JSON marshal errors (not on invalid FBC data)
- [ ] All existing `catalog/fbc/` tests pass after migration
- [ ] Old helper functions are removed

## Project Conventions

- [ ] Package is in `catalog/fbc/internal/testing/catalogfs/`
- [ ] Follows `From/To` and standalone-function conventions where applicable
- [ ] No unnecessary public API surface (internal package)
- [ ] No legacy dependencies added
- [ ] `make ci` passes (lint, test, build)
- [ ] Test coverage does not decrease
