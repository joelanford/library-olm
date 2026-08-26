package v1_test

import (
	"fmt"
	"log"
	"testing/fstest"

	helmv1 "github.com/joelanford/library-olm/bundle/helm/v1"
)

func ExampleFromFS() {
	chart, err := helmv1.FromFS(exampleChartFS())
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("name:", chart.Name())
	fmt.Println("version:", chart.Metadata.Version)
	fmt.Println("templates:", len(chart.Templates))

	// Output:
	// name: my-operator
	// version: 1.0.0
	// templates: 1
}

func ExampleToPlainManifests() {
	chart, err := helmv1.FromFS(exampleChartFS())
	if err != nil {
		log.Fatal(err)
	}

	objects, err := helmv1.ToPlainManifests(chart, "my-operator", "operators")
	if err != nil {
		log.Fatal(err)
	}

	for _, object := range objects {
		fmt.Println(object.GetObjectKind().GroupVersionKind().Kind, object.GetName(), object.GetNamespace())
	}

	// Output:
	// ConfigMap my-operator operators
}

func exampleChartFS() fstest.MapFS {
	return fstest.MapFS{
		"Chart.yaml": {
			Data: []byte("apiVersion: v2\nname: my-operator\nversion: 1.0.0\n"),
		},
		"templates/configmap.yaml": {
			Data: []byte("apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: {{ .Release.Name }}\n  namespace: {{ .Release.Namespace }}\n"),
		},
	}
}
