# Requirements

- `CertManagerCertificateProvider` is accessible as `v1.CertManagerCertificateProvider`
- `OpenshiftServiceCACertificateProvider` is accessible as `v1.OpenshiftServiceCACertificateProvider`
- Both satisfy the `v1.CertificateProvider` interface
- No new packages or constructors introduced
- Internal implementations remain in `internal/render/certproviders/`

## Acceptance Criteria

- A caller outside this module can write `v1.CertManagerCertificateProvider{}` and pass it to `v1.WithCertificateProvider`
- A caller outside this module can write `v1.OpenshiftServiceCACertificateProvider{}` and pass it to `v1.WithCertificateProvider`
- `make ci` passes
