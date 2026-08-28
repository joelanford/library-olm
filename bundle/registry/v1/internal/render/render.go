package render

import (
	"errors"
	"fmt"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/config"
	hashutil "github.com/joelanford/library-olm/bundle/registry/v1/internal/hash"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util"
)

// BundleValidator validates a RegistryV1 bundle by executing a series of
// checks on it and collecting any errors that were found
type BundleValidator []func(v1 *bundle.RegistryV1) []error

func (v BundleValidator) Validate(rv1 *bundle.RegistryV1) error {
	errs := make([]error, 0, len(v))
	for _, validator := range v {
		errs = append(errs, validator(rv1)...)
	}
	return errors.Join(errs...)
}

// GeneratorContext contains state produced while rendering a bundle.
type GeneratorContext struct {
	InstallNamespace string
	Objects          []client.Object
}

// ResourceGenerator generates or transforms resources using read-only bundle and options inputs.
type ResourceGenerator func(bundle.RegistryV1, Options, *GeneratorContext) error

func (g ResourceGenerator) GenerateResources(rv1 bundle.RegistryV1, opts Options, ctx *GeneratorContext) error {
	return g(rv1, opts, ctx)
}

// ResourceGenerators aggregates generators and invokes them in order.
type ResourceGenerators []ResourceGenerator

func (r ResourceGenerators) GenerateResources(rv1 bundle.RegistryV1, opts Options, ctx *GeneratorContext) error {
	for _, generator := range r {
		if err := generator.GenerateResources(rv1, opts, ctx); err != nil {
			return err
		}
	}
	return nil
}

func (r ResourceGenerators) ResourceGenerator() ResourceGenerator {
	return r.GenerateResources
}

type UniqueNameGenerator func(string, interface{}) string

type Options struct {
	TargetNamespaces    []string
	UniqueNameGenerator UniqueNameGenerator
	CertificateProvider CertificateProvider
	// DeploymentConfig contains optional customizations to apply to CSV deployments.
	// If nil, no customizations are applied.
	DeploymentConfig *config.DeploymentConfig

	// SelfManagedInstallNamespace, when non-nil, declares that the caller manages
	// the install namespace of the given name. In that case no Namespace object is
	// generated. When nil, the install namespace is derived from the bundle and a
	// Namespace object is generated.
	SelfManagedInstallNamespace *string
}

func (o *Options) apply(opts ...Option) *Options {
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	return o
}

func (o *Options) validate() (*Options, []error) {
	var errs []error
	if o.UniqueNameGenerator == nil {
		errs = append(errs, errors.New("unique name generator must be specified"))
	}
	return o, errs
}

type Option func(*Options)

// WithTargetNamespaces sets the target namespaces to be used when rendering the bundle
// The value will only be used if len(namespaces) > 0. Otherwise, the default value for the bundle
// derived from its install mode support will be used (if such a value can be defined).
func WithTargetNamespaces(namespaces ...string) Option {
	return func(o *Options) {
		if len(namespaces) > 0 {
			o.TargetNamespaces = namespaces
		}
	}
}

// WithSelfManagedInstallNamespace declares that the caller manages the install
// namespace with the given name. When set, the renderer uses name as the install
// namespace and does not generate a Namespace object. When unset, the renderer
// derives the install namespace from the bundle's CSV annotations and generates
// a Namespace object.
func WithSelfManagedInstallNamespace(name string) Option {
	return func(o *Options) {
		o.SelfManagedInstallNamespace = &name
	}
}

func WithUniqueNameGenerator(generator UniqueNameGenerator) Option {
	return func(o *Options) {
		o.UniqueNameGenerator = generator
	}
}

func WithCertificateProvider(provider CertificateProvider) Option {
	return func(o *Options) {
		o.CertificateProvider = provider
	}
}

// WithDeploymentConfig sets the deployment configuration to apply to CSV deployments.
// If deploymentConfig is nil, no customizations are applied.
func WithDeploymentConfig(deploymentConfig *config.DeploymentConfig) Option {
	return func(o *Options) {
		o.DeploymentConfig = deploymentConfig
	}
}

type BundleRenderer struct {
	BundleValidator    BundleValidator
	ResourceGenerators []ResourceGenerator
}

func (r BundleRenderer) Render(rv1 bundle.RegistryV1, opts ...Option) ([]client.Object, error) {
	// validate bundle
	if err := r.BundleValidator.Validate(&rv1); err != nil {
		return nil, err
	}

	// generate bundle objects
	genOpts := (&Options{
		// default options
		TargetNamespaces:    defaultTargetNamespacesForBundle(&rv1),
		UniqueNameGenerator: DefaultUniqueNameGenerator,
		CertificateProvider: nil,
	}).apply(opts...)

	// validate options
	if _, errs := genOpts.validate(); len(errs) > 0 {
		return nil, fmt.Errorf("invalid option(s): %w", errors.Join(errs...))
	}

	ctx := &GeneratorContext{}
	if err := ResourceGenerators(r.ResourceGenerators).GenerateResources(rv1, *genOpts, ctx); err != nil {
		return nil, err
	}

	return ctx.Objects, nil
}

func DefaultUniqueNameGenerator(base string, o interface{}) string {
	hashStr := hashutil.DeepHashObject(o)
	return util.ObjectNameForBaseAndSuffix(base, hashStr)
}

func defaultTargetNamespacesForBundle(rv1 *bundle.RegistryV1) []string {
	supportedInstallModes := bundle.SupportedInstallModes(*rv1)

	if supportedInstallModes.Has(v1alpha1.InstallModeTypeAllNamespaces) {
		return []string{corev1.NamespaceAll}
	}

	return nil
}
