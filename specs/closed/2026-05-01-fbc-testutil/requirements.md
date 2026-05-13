# Requirements

- Fluent builder API (`Builder().WithPackage().WithChannel().WithBundle().Build()`) that returns `fstest.MapFS`
- `WithPackage(name)` adds an `olm.package` blob
- `WithChannel(pkg, name, entries...)` adds an `olm.channel` blob with entries
- `WithBundle(pkg, version, opts...)` adds an `olm.bundle` blob with `olm.package` property
- `Entry(version, opts...)` creates a channel entry; bundle name derived as `<package>.v<version>`
- `Replaces(version)` entry option; resolved to `<package>.v<version>` during Build
- `Skips(versions...)` entry option; each resolved to `<package>.v<version>` during Build
- `SkipRange(range)` entry option; raw semver range string (not resolved)
- `WithName(name)` bundle option overriding the default bundle name
- `WithImage(image)` bundle option overriding the default image
- `WithRelease(release)` bundle option setting the release field
- Default bundle name: `<package>.v<version>`
- Default bundle image: `quay.io/<package>/bundle:v<version>`
- Output: one file per blob, organized under `<package>/` directories
- `Build()` panics on internal errors (JSON marshal failures)
- Builder is chainable: each `With*` method returns the builder
- Multiple packages supported in a single builder

## Acceptance Criteria

- Builder produces valid FBC JSON parseable by `declcfg.WalkMetasFS`
- All defaults are applied correctly when no overrides are specified
- Each override (`WithName`, `WithImage`, `WithRelease`) changes only its target field
- `Replaces` and `Skips` version strings are correctly resolved to `<package>.v<version>` bundle names
- `SkipRange` is passed through as-is (no version resolution)
- Existing `catalog/fbc/` tests pass when migrated to use the builder
- Old helper functions (`fbcPackage`, `fbcChannel`, `fbcBundle`, etc.) are removed after migration
- `make ci` passes (lint, test, build)
