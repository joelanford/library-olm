# Implementation Plan

## 1. Add VerificationMode type and built-in modes

Add to `image/containers_image_client.go`:

- Define `VerificationMode` as a function type `func(*types.SystemContext) (*signature.PolicyContext, error)`.
- Implement `VerifyAlways`, `VerifyNever`, `VerifyIfPresent` as functions matching the type.
- Implement `VerifyWithPolicy(policy) VerificationMode` that returns a closure.
- Define `Option` function type and private `options` struct with a `getPolicyContext VerificationMode` field.
- Implement `WithSignatureVerification(mode)` option constructor.

## 2. Update constructor to accept options and load policy

Modify `NewContainersImageRepository`:

- Change signature to accept `...Option`.
- Default `getPolicyContext` to `VerifyAlways`.
- Call `cfg.getPolicyContext(srcCtx)` to produce the policy context — before creating the image source.
- If policy loading fails, return the error directly (no cleanup needed since image source wasn't created yet).
- If image source creation fails, destroy the policy context via `errors.Join`.
- Add `policyContext *signature.PolicyContext` field to `ContainersImageRepository`.

## 3. Extract shared getManifest helper

Add a private method to `ContainersImageRepository`:

```go
func (c *ContainersImageRepository) getManifest(ctx context.Context, instanceDigest *digest.Digest) ([]byte, string, error)
```

- Call `c.imageSource.GetManifest(ctx, instanceDigest)`.
- Create `image.UnparsedInstance(c.imageSource, instanceDigest)`.
- Call `c.policyContext.IsRunningImageAllowed(ctx, unparsed)`.
- If rejected, return `fmt.Errorf("image signature verification failed for %s@%s: %w", c.ref.Name(), manifestDigest, err)` — the digest is always available since we just fetched the manifest bytes. If `err` is nil (allowed==false with no error), use a fallback error message.
- Return the manifest bytes and media type.

## 4. Rewrite Resolve and FetchManifest to use the helper

Modify `ContainersImageRepository.Resolve()`:
- Replace the `c.imageSource.GetManifest(ctx, nil)` call with `c.getManifest(ctx, nil)`. The rest (digest computation, descriptor construction) stays the same.

Modify `ContainersImageRepository.FetchManifest()`:
- Replace the body with `return c.getManifest(ctx, &desc.Digest)`.

## 5. Update Close to destroy policy context

Modify `ContainersImageRepository.Close()`:

- Call `policyContext.Destroy()` unconditionally (policy context is always non-nil).
- Use `errors.Join` to combine the destroy error with the image source close error.

## 6. Update test infrastructure

Modify `image/containers_image_client_test.go`:

- Add `fakeTransport` implementing `types.ImageTransport` for policy evaluation.
- Update `fakeImageReference` so that `PolicyConfigurationIdentity()` returns the docker reference name and `PolicyConfigurationNamespaces()` returns the domain. Remove the `transport` field and return `fakeTransport{}` from `Transport()`.
- Wire `fakeImageSource` to store and return its `fakeImageReference` from `Reference()` (previously returned nil).
- Add helper functions: `mustPolicy`, `insecureAcceptAllPolicy`, `rejectAllPolicy`, `skipVerificationPolicyContext`.

## 7. Update existing tests

Update all existing `NewContainersImageRepository` call sites in tests to pass `WithSignatureVerification(VerifyNever)` since the default is now `VerifyAlways` and the test fakes don't have policy files.

Update tests that construct `ContainersImageRepository` directly (bypassing the constructor) to include a `policyContext` field from `skipVerificationPolicyContext(t)`.

## 8. Add new tests

Add test cases to `image/containers_image_client_test.go`:

**Constructor tests:**
- `VerifyAlways` without policy file (explicit nonexistent path): construction fails.
- `VerifyNever`: construction succeeds.
- `VerifyIfPresent` without policy file: construction succeeds.
- `VerifyIfPresent` with invalid policy file: construction fails.
- `VerifyWithPolicy` with a valid policy: construction succeeds.

**Resolve tests:**
- Resolve with accept-all policy: succeeds.
- Resolve with reject-all policy: returns error containing "signature verification failed".

**FetchManifest tests:**
- FetchManifest with accept-all policy: succeeds.
- FetchManifest with reject-all policy: returns error containing "signature verification failed".

## 9. Run CI

Run `make ci` to confirm lint, test, and build all pass.
