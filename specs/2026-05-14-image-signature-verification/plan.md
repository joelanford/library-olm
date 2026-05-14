# Implementation Plan

## 1. Add Option type and signature verification options

Add to `image/containers_image_client.go`:

- Define `Option` as a function type that modifies a private config struct.
- Define `SignatureVerificationMode` enum with `VerifyAlways`, `VerifySkip`, `VerifyIfPresent`.
- Implement `WithSignatureVerification(mode)` and `WithSignatureVerificationPolicy(policy)` option constructors.
- Define the private config struct with fields for mode and optional custom policy.

## 2. Update constructor to accept options and load policy

Modify `NewContainersImageRepository`:

- Change signature to accept `...Option`.
- After creating the image source, process options to determine verification mode.
- Based on mode:
  - `VerifyAlways` (default): call `signature.DefaultPolicy(srcCtx)`, then `signature.NewPolicyContext(policy)`. Return error if either fails.
  - `VerifySkip`: leave `policyContext` nil.
  - `VerifyIfPresent`: call `signature.DefaultPolicy(srcCtx)`. On success, create policy context. On error, check if the error is a `signature.InvalidPolicyFormatError` (policy file found but malformed) — if so, return the error. Otherwise (no policy file found), proceed with `policyContext = nil`.
  - Custom policy provided: call `signature.NewPolicyContext(policy)` with the provided policy. Return error if it fails.
- Add `policyContext *signature.PolicyContext` field to `ContainersImageRepository`.

## 3. Extract shared fetch-and-verify helper

Add a private method to `ContainersImageRepository`:

```go
func (c *ContainersImageRepository) getManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error)
```

- Call `c.imageSource.GetManifest(ctx, instanceDigest)`.
- If `policyContext` is non-nil:
  - Create `image.UnparsedInstance(c.imageSource, instanceDigest)`.
  - Call `c.policyContext.IsRunningImageAllowed(ctx, unparsed)`.
  - If rejected, return `fmt.Errorf("image signature verification failed for %s@%s: %w", c.ref, manifestDigest, err)` — the digest is always available since we just fetched the manifest bytes.
- Return the manifest bytes and media type.

## 4. Rewrite Resolve and FetchManifest to use the helper

Modify `ContainersImageRepository.Resolve()`:
- Replace the `c.imageSource.GetManifest(ctx, nil)` call with `c.getManifest(ctx, nil)`. The rest (digest computation, descriptor construction) stays the same.

Modify `ContainersImageRepository.FetchManifest()`:
- Replace the body with `return c.getManifest(ctx, &desc.Digest)`.

## 5. Update Close to destroy policy context

Modify `ContainersImageRepository.Close()`:

- If `policyContext` is non-nil, call `policyContext.Destroy()`.
- Use `errors.Join` to combine the destroy error with the image source close error.

## 6. Update test infrastructure

Modify `image/containers_image_client_test.go`:

- Update `fakeImageReference` so that `PolicyConfigurationIdentity()` returns a non-empty string derived from the docker reference (e.g., `"docker.io/library/test"`) and `PolicyConfigurationNamespaces()` returns a reasonable hierarchy. This is required for policy rule matching.
- Wire `fakeImageSource` to store and return its `fakeImageReference` from `Reference()` (currently returns nil).
- Add a helper function that creates a `*signature.Policy` from a JSON string using `signature.NewPolicyFromBytes()`, for use in `WithSignatureVerificationPolicy()` tests.

## 7. Update existing tests

Update all existing `NewContainersImageRepository` call sites in tests to pass `WithSignatureVerification(VerifySkip)` since the default is now `VerifyAlways` and the test fakes don't have policy files. This maintains existing test behavior.

Update tests that construct `ContainersImageRepository` directly (bypassing the constructor) — these are unaffected since they don't set `policyContext`.

## 8. Add new tests

Add test cases to `image/containers_image_client_test.go`:

**Constructor tests:**
- `VerifyAlways` with valid policy (via `WithSignatureVerificationPolicy`): construction succeeds, `policyContext` is set.
- `VerifyAlways` without policy file (default, no `SystemContext`): construction fails.
- `VerifySkip`: construction succeeds, `policyContext` is nil.
- `VerifyIfPresent` without policy file: construction succeeds, `policyContext` is nil.
- `VerifyIfPresent` with invalid policy file: construction fails.
- `WithSignatureVerificationPolicy` with a valid policy: construction succeeds.

**Resolve tests:**
- Resolve with accept-all policy: succeeds.
- Resolve with reject-all policy: returns error containing "signature verification failed".
- Resolve without policy context (VerifySkip): succeeds (no verification).

**FetchManifest tests:**
- FetchManifest with accept-all policy: succeeds.
- FetchManifest with reject-all policy: returns error containing "signature verification failed".
- FetchManifest without policy context (VerifySkip): succeeds (no verification).

**Close tests:**
- Close with policy context: succeeds, destroys context.
- Close without policy context: succeeds.

## 9. Run CI

Run `make ci` to confirm lint, test, and build all pass.
