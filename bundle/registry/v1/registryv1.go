package v1

import (
	"io/fs"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle/source"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/config"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render/certproviders"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render/registryv1"
)

// Bundle is a parsed registry+v1 bundle containing a CSV, CRDs, and other resources.
type Bundle = bundle.RegistryV1

// Config holds validated configuration for a registry+v1 bundle.
type Config = config.Config

// CertificateProvider handles certificate provisioning for webhook resources.
type CertificateProvider = render.CertificateProvider

// CertManagerCertificateProvider is a CertificateProvider that uses cert-manager to provision
// TLS certificates for webhook services.
type CertManagerCertificateProvider = certproviders.CertManagerCertificateProvider

// OpenshiftServiceCACertificateProvider is a CertificateProvider that uses the OpenShift
// service CA operator to provision TLS certificates for webhook services.
type OpenshiftServiceCACertificateProvider = certproviders.OpenshiftServiceCaCertificateProvider

// DeploymentConfig contains optional customizations to apply to CSV deployments.
type DeploymentConfig = config.DeploymentConfig

// RenderOption configures rendering behavior.
type RenderOption = render.Option

// FromFS reads a registry+v1 bundle from the given filesystem and returns a Bundle.
// The filesystem is expected to contain metadata/annotations.yaml and a manifests/ directory.
func FromFS(bundleFS fs.FS) (Bundle, error) {
	return source.FromFS(bundleFS).GetBundle()
}

// ValidateConfig validates raw user configuration (YAML or JSON) against the given
// schema and install namespace constraints. The schema is typically obtained from
// [Bundle.GetConfigSchema] and may be modified before validation (e.g., to remove
// fields gated behind feature flags).
var ValidateConfig = config.UnmarshalConfig

// Validate checks a parsed registry+v1 Bundle for structural and semantic correctness.
func Validate(b Bundle) error {
	return registryv1.BundleValidator.Validate(&b)
}

// ToPlainManifests converts a parsed registry+v1 Bundle into plain Kubernetes manifests.
//
// By default the install namespace is derived from the bundle's CSV annotations
// (operatorframework.io/suggested-namespace-template, then
// operatorframework.io/suggested-namespace, then "<PackageName>-system") and a
// Namespace object for it is included as the first manifest. Pass
// [WithSelfManagedInstallNamespace] to use a caller-managed namespace instead, in
// which case no Namespace object is emitted.
func ToPlainManifests(b Bundle, opts ...RenderOption) ([]client.Object, error) {
	return registryv1.Renderer.Render(b, opts...)
}

var (
	// WithTargetNamespaces sets the target namespaces for the rendered bundle.
	WithTargetNamespaces = render.WithTargetNamespaces

	// WithCertificateProvider sets the certificate provider for webhook resources.
	WithCertificateProvider = render.WithCertificateProvider

	// WithDeploymentConfig sets deployment customizations to apply to CSV deployments.
	WithDeploymentConfig = render.WithDeploymentConfig

	// WithSelfManagedInstallNamespace declares that the caller manages the install
	// namespace with the given name. When set, no Namespace object is generated and
	// the given name is used as the install namespace. When unset, the install
	// namespace is derived from the bundle and a Namespace object is generated.
	WithSelfManagedInstallNamespace = render.WithSelfManagedInstallNamespace
)
