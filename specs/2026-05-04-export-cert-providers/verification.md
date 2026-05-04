# Verification

## Implementation Correctness

- [ ] Both types are exported as type aliases (not defined types) to preserve identity with internal types
- [ ] Both types satisfy `v1.CertificateProvider` (compile-time check via existing interface assertions in internal package)
- [ ] `make ci` passes

## Project Conventions

- [ ] Minimal public API surface — only two type aliases added, no new packages
- [ ] Internal implementations remain in `internal/render/certproviders/`
- [ ] Commit uses conventional commit format (`feat:`)
