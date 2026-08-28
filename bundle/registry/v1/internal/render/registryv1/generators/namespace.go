package generators

import (
	"errors"
	"fmt"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render"
)

const (
	// annotationSuggestedNamespaceTemplate is a CSV annotation whose value is a
	// JSON/YAML-encoded corev1.Namespace. When present, it is authoritative: the
	// install namespace is taken from the template's metadata.name and the whole
	// object (labels, annotations, etc.) is emitted.
	annotationSuggestedNamespaceTemplate = "operatorframework.io/suggested-namespace-template"

	// annotationSuggestedNamespace is a CSV annotation whose value is the
	// suggested install namespace name. Used only when the template annotation is
	// absent.
	annotationSuggestedNamespace = "operatorframework.io/suggested-namespace"
)

// BundleNamespaceGenerator resolves the install namespace before resource generation.
func BundleNamespaceGenerator(rv1 bundle.RegistryV1, opts render.Options, ctx *render.GeneratorContext) error {
	ns, emit, err := resolveInstallNamespace(&rv1, opts.SelfManagedInstallNamespace)
	if err != nil {
		return err
	}
	if err := validateTargetNamespaces(&rv1, ns.Name, opts.TargetNamespaces); err != nil {
		return fmt.Errorf("invalid option(s): invalid target namespaces %v: %w", opts.TargetNamespaces, err)
	}

	ctx.InstallNamespace = ns.Name
	if emit {
		ctx.Objects = append(ctx.Objects, &ns)
	}
	return nil
}

func validateTargetNamespaces(rv1 *bundle.RegistryV1, installNamespace string, targetNamespaces []string) error {
	supportedInstallModes := bundle.SupportedInstallModes(*rv1)

	set := sets.New[string](targetNamespaces...)
	switch {
	case set.Len() == 0:
		// Note: this function generally expects targetNamespace to contain at least one value set by default
		// in case the user does not specify the value. The option to set the targetNamespace is a no-op if it is empty.
		// The only case for which a default targetNamespace is undefined is in the case of a bundle that only
		// supports SingleNamespace install mode. The if statement here is added to provide a more friendly error
		// message than just the generic (at least one target namespace must be specified) which would occur
		// in case only the MultiNamespace install mode is supported by the bundle.
		// If AllNamespaces mode is supported, the default will be [""] -> watch all namespaces
		// If only OwnNamespace is supported, the default will be [install-namespace] -> only watch the install/own namespace
		if supportedInstallModes.Has(v1alpha1.InstallModeTypeMultiNamespace) {
			return errors.New("at least one target namespace must be specified")
		}
		return errors.New("exactly one target namespace must be specified")
	case set.Len() == 1 && set.Has(""):
		if supportedInstallModes.Has(v1alpha1.InstallModeTypeAllNamespaces) {
			return nil
		}
		return fmt.Errorf("supported install modes %v do not support targeting all namespaces", sets.List(supportedInstallModes))
	case set.Len() == 1 && !set.Has(""):
		if targetNamespaces[0] == installNamespace {
			if !supportedInstallModes.Has(v1alpha1.InstallModeTypeOwnNamespace) {
				return fmt.Errorf("supported install modes %v do not support targeting own namespace", sets.List(supportedInstallModes))
			}
			return nil
		}
		if supportedInstallModes.Has(v1alpha1.InstallModeTypeSingleNamespace) {
			return nil
		}
	default:
		if !supportedInstallModes.Has(v1alpha1.InstallModeTypeOwnNamespace) && set.Has(installNamespace) {
			return fmt.Errorf("supported install modes %v do not support targeting own namespace", sets.List(supportedInstallModes))
		}
		if supportedInstallModes.Has(v1alpha1.InstallModeTypeMultiNamespace) && !set.Has("") {
			return nil
		}
	}
	return fmt.Errorf("supported install modes %v do not support target namespaces %v", sets.List[v1alpha1.InstallModeType](supportedInstallModes), targetNamespaces)
}

// resolveInstallNamespace determines the install namespace for a bundle render.
// It returns the resolved Namespace (whose Name is the install namespace) and
// whether that Namespace object should be emitted into the rendered manifests.
//
// When selfManaged is non-nil, the caller owns the namespace: its name is used
// as the install namespace and emit is false.
//
// Otherwise a full corev1.Namespace is resolved from the CSV annotations, in
// precedence order, and emit is true:
//  1. operatorframework.io/suggested-namespace-template - authoritative when
//     present; unmarshalled directly into a Namespace.
//  2. operatorframework.io/suggested-namespace - used only when the template
//     annotation is absent.
//  3. <PackageName>-system - used only when both annotations are absent.
//
// The resolved name is validated in all cases; an invalid name (including an
// empty template name) is a hard error rather than a fallthrough.
func resolveInstallNamespace(rv1 *bundle.RegistryV1, selfManaged *string) (namespace corev1.Namespace, emit bool, err error) {
	if selfManaged != nil {
		if err := validateNamespaceName(*selfManaged); err != nil {
			return corev1.Namespace{}, false, fmt.Errorf("invalid self-managed install namespace: %w", err)
		}
		return *newNamespace(*selfManaged), false, nil
	}
	ns, err := deriveNamespace(rv1)
	if err != nil {
		return corev1.Namespace{}, false, err
	}
	if err := validateNamespaceName(ns.Name); err != nil {
		return corev1.Namespace{}, false, fmt.Errorf("invalid install namespace: %w", err)
	}
	return *ns, true, nil
}

// deriveNamespace builds the Namespace object to emit from the bundle's CSV
// annotations, following the resolution precedence. It does not validate the
// resulting name.
func deriveNamespace(rv1 *bundle.RegistryV1) (*corev1.Namespace, error) {
	annotations := rv1.CSV.GetAnnotations()
	if tmpl, ok := annotations[annotationSuggestedNamespaceTemplate]; ok {
		ns := &corev1.Namespace{}
		if err := yaml.Unmarshal([]byte(tmpl), ns); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s: %w", annotationSuggestedNamespaceTemplate, err)
		}
		return withNamespaceTypeMeta(ns), nil
	}
	if name, ok := annotations[annotationSuggestedNamespace]; ok {
		return newNamespace(name), nil
	}
	return newNamespace(fmt.Sprintf("%s-system", rv1.PackageName)), nil
}

// newNamespace constructs a bare Namespace object with only its name set.
func newNamespace(name string) *corev1.Namespace {
	return withNamespaceTypeMeta(&corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	})
}

// withNamespaceTypeMeta forces the correct GroupVersionKind on a Namespace so
// the emitted object is self-describing regardless of what a template blob
// specified.
func withNamespaceTypeMeta(ns *corev1.Namespace) *corev1.Namespace {
	ns.TypeMeta = metav1.TypeMeta{
		Kind:       "Namespace",
		APIVersion: corev1.SchemeGroupVersion.String(),
	}
	return ns
}

// validateNamespaceName checks that name is a valid Kubernetes namespace name.
// IsDNS1123Label enforces non-emptiness, the <=63 character limit, and the label
// format.
func validateNamespaceName(name string) error {
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		return fmt.Errorf("invalid namespace name %q: %v", name, errs)
	}
	return nil
}
