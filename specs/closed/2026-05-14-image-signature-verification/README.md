---
status: done
pr: https://github.com/joelanford/library-olm/pull/16
---
# Image Signature Verification

## Summary

Add policy-based image signature verification to `ContainersImageRepository` using the containers/image `signature.PolicyContext` API. Verification is controlled via functional options on the constructor and enforced on every manifest encountered — both the top-level manifest during `Resolve()` and each child manifest fetched via `FetchManifest()`. If verification fails, the operation returns an error before any content is made available.

This connects to mission goal #2 (distribution libraries) — signature verification is a core security concern when reading images from OCI registries.

## Design

### How containers/image signature verification works

The containers/image library provides a policy-based signature verification system:

1. **Policy** (`signature.Policy`) — loaded from a `policy.json` file (located via `SystemContext.SignaturePolicyPath` or system defaults). Contains rules mapping image references to verification requirements (e.g., "accept anything", "reject", "require sigstore signature from key X").

2. **PolicyContext** (`signature.PolicyContext`) — created from a Policy. Provides `IsRunningImageAllowed(ctx, unparsedImage)` which evaluates the policy rules against an image's reference, manifest, and signatures.

3. **UnparsedImage** (`image.UnparsedInstance(src, instanceDigest)`) — wraps an `ImageSource` to provide manifest bytes and signatures for a specific manifest. When `instanceDigest` is nil, it represents the top-level manifest (could be an index or single image). When non-nil, it represents a specific child manifest from an index.

### Functional options API

Signature verification is configured via functional options on `NewContainersImageRepository`:

```go
// VerificationMode produces a PolicyContext from a SystemContext.
type VerificationMode func(*types.SystemContext) (*signature.PolicyContext, error)

func VerifyAlways(sysCtx) ...    // load from SystemContext, fail if missing
func VerifyNever(sysCtx) ...     // accept-all policy, no verification
func VerifyIfPresent(sysCtx) ... // load if found, accept-all if missing, fail on parse error
func VerifyWithPolicy(policy) VerificationMode // use a caller-provided policy

func WithSignatureVerification(mode VerificationMode) Option
```

Usage:
```go
// Default: must have policy — construction fails without one
NewContainersImageRepository(ctx, ref, sysCtx)

// Never verify
NewContainersImageRepository(ctx, ref, sysCtx, WithSignatureVerification(VerifyNever))

// Verify if a policy file exists, skip if not
NewContainersImageRepository(ctx, ref, sysCtx, WithSignatureVerification(VerifyIfPresent))

// Use a caller-provided policy
NewContainersImageRepository(ctx, ref, sysCtx, WithSignatureVerification(VerifyWithPolicy(myPolicy)))
```

### Integration into ContainersImageRepository

The constructor calls `cfg.getPolicyContext(srcCtx)` to produce a `*signature.PolicyContext`, which is always non-nil (even `VerifyNever` produces an accept-all context). This eliminates nil checks throughout.

**Shared helper:** A private `getManifest(ctx, instanceDigest)` method handles both fetching and verification. It calls `imageSource.GetManifest`, then creates `image.UnparsedInstance(imageSource, instanceDigest)` and calls `IsRunningImageAllowed()`. Both `GetManifest` and `UnparsedInstance` take the same `*digest.Digest` parameter — nil for the top-level, non-nil for children.

**Resolve():** Calls `getManifest(ctx, nil)`, then computes the digest and builds the descriptor from the result.

**FetchManifest():** Delegates directly to `getManifest(ctx, &desc.Digest)`.

**Close():** Always calls `policyContext.Destroy()` alongside `imageSource.Close()`.

### Verification scope

This is stricter than podman/skopeo, which only verify the selected child manifest from an index (not the index itself). Our implementation verifies every manifest encountered:
- Top-level at `Resolve()` (whether it's an index or single image)
- Each child at `FetchManifest()` when fetching by digest

### Error wrapping

The `getManifest` helper computes the manifest digest from the fetched bytes, so verification errors always include both the reference and digest: `"image signature verification failed for %s@%s: %w"`.

### Test strategy

Tests use `VerifyWithPolicy(policy)` with programmatically constructed policies:
- `insecureAcceptAnything` default rule → accept all images
- `reject` default rule → reject all images

The existing `fakeImageSource` and `fakeImageReference` test infrastructure is extended to support policy evaluation (the `fakeImageReference` returns valid `PolicyConfigurationIdentity` values and `fakeImageSource.Reference()` returns the reference).
