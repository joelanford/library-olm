# Verification

## Implementation Correctness

- [ ] `VerificationMode` function type and built-in modes (`VerifyAlways`, `VerifyNever`, `VerifyIfPresent`, `VerifyWithPolicy`) are defined and exported.
- [ ] `Option` type and `WithSignatureVerification` are defined and exported.
- [ ] `NewContainersImageRepository` accepts `...Option` and defaults to `VerifyAlways`.
- [ ] `VerifyAlways` loads policy via `DefaultPolicy(sysCtx)` and fails if no policy is available.
- [ ] `VerifyNever` creates an accept-all policy context (never nil).
- [ ] `VerifyIfPresent` loads policy when available, uses accept-all when no file exists, fails on parse errors or permission errors.
- [ ] `VerifyWithPolicy(policy)` uses the provided policy directly.
- [ ] Policy context is loaded before image source is created.
- [ ] `getManifest` helper handles both fetching and verification in one method.
- [ ] `Resolve()` delegates to `getManifest(ctx, nil)`.
- [ ] `FetchManifest()` delegates to `getManifest(ctx, &desc.Digest)`.
- [ ] `getManifest` returns an error when the policy rejects the image.
- [ ] `getManifest` provides a fallback error message when `IsRunningImageAllowed` returns `allowed==false` with `err==nil`.
- [ ] `Close()` calls `PolicyContext.Destroy()` unconditionally (policy context is always non-nil).
- [ ] Error messages from `getManifest` include `ref.Name()` and the manifest digest.
- [ ] Existing tests updated with `VerifyNever` continue to pass.

## Project Conventions

- [ ] `Repository` interface is unchanged — no new methods added.
- [ ] New public API is limited to `VerificationMode`, `Option`, mode functions, and `WithSignatureVerification` — all on the `image` package, not the interface.
- [ ] Uses `containers/image` library (`go.podman.io/image/v5`) for policy evaluation — no new dependencies added.
- [ ] Follows "pure data types with standalone functions" principle — policy context is an implementation detail.
- [ ] Error wrapping uses `fmt.Errorf` with `%w` for error chaining.
- [ ] No `//nolint` comments added.
- [ ] Test coverage for new code meets the 70% statement coverage minimum.
- [ ] Commit messages follow conventional commits format.
- [ ] `make ci` passes (lint + test + build).
