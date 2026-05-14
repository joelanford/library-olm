---
status: in-progress
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
type SignatureVerificationMode int

const (
    // VerifyAlways loads policy from SystemContext.
    // Construction fails if no policy file is found or parsing fails.
    VerifyAlways SignatureVerificationMode = iota

    // VerifySkip disables signature verification entirely.
    VerifySkip

    // VerifyIfPresent verifies when a policy file is found,
    // skips if no file exists. Fails if the file exists but can't be parsed.
    VerifyIfPresent
)

// WithSignatureVerification sets the signature verification mode.
// Default (when not called) is VerifyAlways.
func WithSignatureVerification(mode SignatureVerificationMode) Option

// WithSignatureVerificationPolicy provides a custom policy, bypassing
// the SystemContext file lookup. Implies VerifyAlways.
func WithSignatureVerificationPolicy(policy *signature.Policy) Option
```

Usage:
```go
// Default: must have policy — construction fails without one
NewContainersImageRepository(ctx, ref, sysCtx)

// Skip verification entirely
NewContainersImageRepository(ctx, ref, sysCtx, WithSignatureVerification(VerifySkip))

// Verify if a policy file exists, skip if not
NewContainersImageRepository(ctx, ref, sysCtx, WithSignatureVerification(VerifyIfPresent))

// Use a caller-provided policy
NewContainersImageRepository(ctx, ref, sysCtx, WithSignatureVerificationPolicy(myPolicy))
```

### Integration into ContainersImageRepository

The constructor signature changes to accept variadic options:

```go
func NewContainersImageRepository(ctx context.Context, imgRef types.ImageReference, srcCtx *types.SystemContext, opts ...Option) (*ContainersImageRepository, error)
```

The struct gains a `policyContext` field:

```go
type ContainersImageRepository struct {
    ref           reference.Named
    imageSource   types.ImageSource
    policyContext *signature.PolicyContext  // nil when verification is disabled
}
```

**Construction:** After creating the image source, policy is loaded based on the mode:
- `VerifyAlways` (default): call `signature.DefaultPolicy(srcCtx)`, then `signature.NewPolicyContext(policy)`. Fail if either step errors.
- `VerifySkip`: no policy loaded, `policyContext` remains nil.
- `VerifyIfPresent`: call `signature.DefaultPolicy(srcCtx)`. If it succeeds, create the policy context. If it fails because no file was found, proceed with `policyContext = nil`. If it fails for any other reason (e.g., parse error), fail construction.
- `WithSignatureVerificationPolicy(policy)`: use the provided policy directly, call `NewPolicyContext(policy)`. Fail if it errors.

**Shared helper:** A private `getManifest(ctx, instanceDigest)` method handles both fetching and verification. It calls `imageSource.GetManifest`, then (when `policyContext` is non-nil) creates `image.UnparsedInstance(imageSource, instanceDigest)` and calls `IsRunningImageAllowed()`. Both `GetManifest` and `UnparsedInstance` take the same `*digest.Digest` parameter — nil for the top-level, non-nil for children.

**Resolve():** Calls `getManifest(ctx, nil)`, then computes the digest and builds the descriptor from the result.

**FetchManifest():** Delegates directly to `getManifest(ctx, &desc.Digest)`.

**Close():** Destroy the policy context (if non-nil) alongside closing the image source.

### Verification scope

This is stricter than podman/skopeo, which only verify the selected child manifest from an index (not the index itself). Our implementation verifies every manifest encountered:
- Top-level at `Resolve()` (whether it's an index or single image)
- Each child at `FetchManifest()` when fetching by digest

### Error wrapping

The `getManifest` helper computes the manifest digest from the fetched bytes, so verification errors always include both the reference and digest: `"image signature verification failed for %s@%s: %w"`.

### Test strategy

Tests use `WithSignatureVerificationPolicy(policy)` with programmatically constructed policies:
- `insecureAcceptAnything` default rule → accept all images
- `reject` default rule → reject all images

The existing `fakeImageSource` and `fakeImageReference` test infrastructure is extended to support policy evaluation (the `fakeImageReference` must return valid `PolicyConfigurationIdentity` values and `fakeImageSource.Reference()` must return the reference, not nil).
