# Requirements

- `NewContainersImageRepository` accepts variadic `Option` arguments to configure signature verification.
- The default behavior (no options) is `VerifyAlways`: load the policy from `SystemContext` via `signature.DefaultPolicy(sysCtx)` and create a `PolicyContext`. Construction fails if no policy file is found or the file can't be parsed.
- `WithSignatureVerification(VerifyNever)` disables signature verification entirely. An accept-all policy context is created so that `policyContext` is never nil.
- `WithSignatureVerification(VerifyIfPresent)` loads the policy if a file is found, uses an accept-all policy if no file exists. Fails construction if the file exists but can't be parsed.
- `WithSignatureVerification(VerifyWithPolicy(policy))` uses the provided `*signature.Policy` directly, bypassing file lookup.
- `VerificationMode` is a function type `func(*types.SystemContext) (*signature.PolicyContext, error)` — the built-in modes are `VerifyAlways`, `VerifyNever`, `VerifyIfPresent`, and `VerifyWithPolicy`.
- `Resolve()` verifies the top-level manifest (index or single image) against the policy before returning the descriptor. Verification uses `image.UnparsedInstance(imageSource, nil)` and `PolicyContext.IsRunningImageAllowed()`.
- `FetchManifest()` verifies each manifest fetched by digest against the policy before returning the bytes. Verification uses `image.UnparsedInstance(imageSource, &digest)`.
- Both `Resolve()` and `FetchManifest()` delegate to a shared `getManifest` helper that handles fetching and verification.
- If the policy rejects an image, the operation returns an error wrapping the policy rejection reason. No manifest bytes or descriptors are returned for rejected images.
- `Close()` destroys the `PolicyContext` (via `Destroy()`) in addition to closing the image source. The policy context is always non-nil so no nil check is needed.
- The `Repository` interface is unchanged — no new methods or types are added to the interface.
- The `VerificationMode` type, `Option` type, mode functions, and `WithSignatureVerification` are new public API surface on the `image` package (not on the interface).

## Acceptance Criteria

- Construction with no options and a valid policy file succeeds and enables verification.
- Construction with no options and no policy file fails with a clear error.
- Construction with `VerifyNever` succeeds regardless of policy file presence.
- Construction with `VerifyIfPresent` and no policy file succeeds without verification.
- Construction with `VerifyIfPresent` and an invalid policy file fails with a parse error.
- Construction with `VerifyWithPolicy(policy)` succeeds and uses the provided policy.
- `Resolve()` succeeds when the policy accepts the image (`insecureAcceptAnything`).
- `Resolve()` returns an error when the policy rejects the image (`reject`).
- `FetchManifest()` succeeds when the policy accepts the child manifest.
- `FetchManifest()` returns an error when the policy rejects the child manifest.
- `Close()` destroys the policy context without error.
- Existing callers that pass nil for `SystemContext` need to add `WithSignatureVerification(VerifyNever)` or provide a policy file — this is an intentional breaking change for security.
