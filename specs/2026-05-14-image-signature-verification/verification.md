# Verification

## Implementation Correctness

- [ ] `Option` type, `SignatureVerificationMode` enum, and option constructors are defined and exported.
- [ ] `NewContainersImageRepository` accepts `...Option` and processes them correctly.
- [ ] Default mode (`VerifyAlways`) loads policy via `DefaultPolicy(sysCtx)` and fails if no policy is available.
- [ ] `VerifySkip` mode skips all policy loading and leaves `policyContext` nil.
- [ ] `VerifyIfPresent` mode loads policy when available, skips when no file exists, fails on parse errors.
- [ ] `WithSignatureVerificationPolicy(policy)` uses the provided policy directly.
- [ ] `Resolve()` calls `IsRunningImageAllowed` on the top-level `UnparsedInstance(src, nil)` before returning.
- [ ] `Resolve()` returns an error when the policy rejects the image.
- [ ] `Resolve()` succeeds without verification when `policyContext` is nil.
- [ ] `FetchManifest()` calls `IsRunningImageAllowed` on `UnparsedInstance(src, &digest)` before returning.
- [ ] `FetchManifest()` returns an error when the policy rejects the child manifest.
- [ ] `FetchManifest()` succeeds without verification when `policyContext` is nil.
- [ ] `Close()` calls `PolicyContext.Destroy()` when a policy context exists.
- [ ] `Close()` succeeds when no policy context exists (nil case).
- [ ] Error messages from `getManifest` always include both the image reference and the manifest digest.
- [ ] Existing tests updated with `VerifySkip` continue to pass.

## Project Conventions

- [ ] `Repository` interface is unchanged — no new methods added.
- [ ] New public API is limited to `Option`, `SignatureVerificationMode`, and option constructors — all on the `image` package, not the interface.
- [ ] Uses `containers/image` library (`go.podman.io/image/v5`) for policy evaluation — no new dependencies added.
- [ ] Follows "pure data types with standalone functions" principle — policy context is an implementation detail.
- [ ] Error wrapping uses `fmt.Errorf` with `%w` for error chaining.
- [ ] No `//nolint` comments added.
- [ ] Test coverage for new code meets the 70% statement coverage minimum.
- [ ] Commit messages follow conventional commits format.
- [ ] `make ci` passes (lint + test + build).
