package render

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
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
