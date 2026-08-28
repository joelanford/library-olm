package render_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render"
)

func Test_WebhookServiceNameForDeployment(t *testing.T) {
	for _, tc := range []struct {
		name           string
		deploymentName string
		expected       string
	}{
		{
			name:           "replaces dots",
			deploymentName: "my.deployment.thing",
			expected:       "my-deployment-thing-service",
		},
		{
			name:           "truncates long names",
			deploymentName: "my.object.thing.has.a.really.really.really.really.really.long.name",
			expected:       "my-object-thing-has-a-really-really-really-really-reall-service",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, render.WebhookServiceNameForDeployment(tc.deploymentName))
		})
	}
}
