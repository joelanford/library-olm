package validate

import (
	"errors"
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// annotationAllowList is intentionally nil: no Helm annotations are allowed.
var annotationAllowList map[string]struct{}

var labelAllowList = map[string]struct{}{
	"helm.sh/chart": {},
}

// HelmMetadata rejects non-allowlisted helm.sh labels and annotations.
func HelmMetadata(objects []client.Object) error {
	var objErrs []error
	for _, obj := range objects {
		for key := range obj.GetAnnotations() {
			if strings.HasPrefix(key, "helm.sh/") {
				if _, allowed := annotationAllowList[key]; !allowed {
					objErrs = append(objErrs, fmt.Errorf("object %s %q has disallowed annotation %q", obj.GetObjectKind().GroupVersionKind(), obj.GetName(), key))
				}
			}
		}
		for key := range obj.GetLabels() {
			if strings.HasPrefix(key, "helm.sh/") {
				if _, allowed := labelAllowList[key]; !allowed {
					objErrs = append(objErrs, fmt.Errorf("object %s %q has disallowed label %q", obj.GetObjectKind().GroupVersionKind(), obj.GetName(), key))
				}
			}
		}
	}
	return errors.Join(objErrs...)
}
