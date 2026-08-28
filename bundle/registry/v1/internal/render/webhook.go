package render

import (
	"strings"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util"
)

// WebhookServiceNameForDeployment returns the OLMv0-compatible Service name
// for a webhook deployment.
//
// This maintains parity with OLMv0 naming.
// See https://github.com/operator-framework/operator-lifecycle-manager/blob/658a6a60de8315f055f54aa7e50771ee4daa8983/pkg/controller/install/webhook.go#L254
func WebhookServiceNameForDeployment(deploymentName string) string {
	return util.ObjectNameForBaseAndSuffix(strings.ReplaceAll(deploymentName, ".", "-"), "service")
}
