package v1_test

import (
	"fmt"
	"log"
	"slices"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	registryv1 "github.com/joelanford/library-olm/bundle/registry/v1"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing/bundlefs"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util/testing/clusterserviceversion"
)

func ExampleFromFS() {
	fs := bundlefs.Builder().
		WithPackageName("my-operator").
		WithBundleResource("csv.yaml", ptr.To(clusterserviceversion.Builder().
			WithName("my-operator.v1.0.0").
			WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
			Build())).
		Build()

	bundle, err := registryv1.FromFS(fs)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("package:", bundle.PackageName)
	fmt.Println("csv:", bundle.CSV.Name)
	fmt.Println("crds:", len(bundle.CRDs))
	fmt.Println("others:", len(bundle.Others))

	// Output:
	// package: my-operator
	// csv: my-operator.v1.0.0
	// crds: 0
	// others: 0
}

func ExampleToPlainManifests() {
	csv := clusterserviceversion.Builder().
		WithName("my-operator.v1.0.0").
		WithInstallModeSupportFor(v1alpha1.InstallModeTypeAllNamespaces).
		WithStrategyDeploymentSpecs(v1alpha1.StrategyDeploymentSpec{
			Name: "controller-manager",
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "manager", Image: "example.com/operator:v1"}},
					},
				},
			},
		}).
		Build()

	fs := bundlefs.Builder().
		WithPackageName("my-operator").
		WithBundleResource("csv.yaml", ptr.To(csv)).
		Build()

	bundle, err := registryv1.FromFS(fs)
	if err != nil {
		log.Fatal(err)
	}

	objs, err := registryv1.ToPlainManifests(bundle, registryv1.WithSelfManagedInstallNamespace("operators"))
	if err != nil {
		log.Fatal(err)
	}

	var kinds []string
	for _, obj := range objs {
		kinds = append(kinds, obj.GetObjectKind().GroupVersionKind().Kind)
	}
	slices.Sort(kinds)
	fmt.Println(kinds)

	// Output:
	// [Deployment]
}
