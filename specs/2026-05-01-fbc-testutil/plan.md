# Implementation Plan

1. **Create the `catalogfs` package with types and builder**
   - Create `catalog/fbc/internal/testing/catalogfs/catalogfs.go`
   - Define `CatalogFSBuilder` interface, `ChannelEntry` type, option types (`EntryOption`, `BundleOption`)
   - Implement the builder struct with `WithPackage`, `WithChannel`, `WithBundle` methods
   - Implement `Build()` to produce `fstest.MapFS` with `catalog.json` containing newline-delimited JSON blobs
   - Implement `Entry()`, `Replaces()`, `Skips()`, `SkipRange()` functions
   - Implement `WithName()`, `WithImage()`, `WithRelease()` bundle options

2. **Write tests for the builder**
   - Create `catalog/fbc/internal/testing/catalogfs/catalogfs_test.go`
   - Test default derivation: bundle name, image, entry resolution
   - Test each override: `WithName`, `WithImage`, `WithRelease`
   - Test entry options: `Replaces`, `Skips`, `SkipRange`
   - Test multiple packages in a single builder
   - Test that output is parseable by `declcfg.WalkMetasFS`

3. **Migrate existing catalog tests to use the builder**
   - Replace `validCatalogFS()` with builder usage
   - Replace `skipRangeCatalogFS()` with builder usage
   - Migrate individual test cases that use `fbcPackage`/`fbcChannel`/`fbcBundle` helpers where the builder can express the test data
   - Leave tests that need malformed JSON structure using raw `fstest.MapFS`
   - Remove old helper functions (`fbcPackage`, `fbcChannel`, `fbcBundle`, `fbcBundleWithRelease`, `fbcBundleNoImage`)
