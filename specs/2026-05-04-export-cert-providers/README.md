---
status: done
---
# Export built-in CertificateProvider implementations

## Summary

The `CertificateProvider` interface is already part of the public API in `bundle/registry/v1`,
but the two built-in implementations (`CertManagerCertificateProvider` and
`OpenshiftServiceCaCertificateProvider`) live in an internal package. Consumers cannot use them
without copying or reimplementing the logic. Export both as type aliases so callers can
instantiate them directly.

## Design

Both implementations are zero-field structs with no constructor logic, so type aliases in
`bundle/registry/v1/registryv1.go` are sufficient — no new packages or factory functions needed.

```go
type CertManagerCertificateProvider = certproviders.CertManagerCertificateProvider
type OpenshiftServiceCACertificateProvider = certproviders.OpenshiftServiceCaCertificateProvider
```

Callers use them as:

```go
v1.ToPlainManifests(bundle, ns,
    v1.WithCertificateProvider(v1.CertManagerCertificateProvider{}))
```

The internal implementations remain in `internal/render/certproviders/` — only the type names
are re-exported.
