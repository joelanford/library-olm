package testing

import (
	"testing"

	"github.com/joelanford/library-olm/bundle/registry/v1/internal/bundle"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/render"
	"github.com/joelanford/library-olm/bundle/registry/v1/internal/util"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type FakeCertProvider struct {
	InjectCABundleFn    func(obj client.Object, cfg render.CertificateProvisionerConfig) error
	AdditionalObjectsFn func(cfg render.CertificateProvisionerConfig) ([]unstructured.Unstructured, error)
	GetCertSecretInfoFn func(cfg render.CertificateProvisionerConfig) render.CertSecretInfo
}

func (f FakeCertProvider) InjectCABundle(obj client.Object, cfg render.CertificateProvisionerConfig) error {
	return f.InjectCABundleFn(obj, cfg)
}

func (f FakeCertProvider) AdditionalObjects(cfg render.CertificateProvisionerConfig) ([]unstructured.Unstructured, error) {
	return f.AdditionalObjectsFn(cfg)
}

func (f FakeCertProvider) GetCertSecretInfo(cfg render.CertificateProvisionerConfig) render.CertSecretInfo {
	return f.GetCertSecretInfoFn(cfg)
}

type FakeBundleSource func() (bundle.RegistryV1, error)

func (f FakeBundleSource) GetBundle() (bundle.RegistryV1, error) {
	return f()
}

func ToUnstructuredT(t *testing.T, obj client.Object) *unstructured.Unstructured {
	u, err := util.ToUnstructured(obj)
	require.NoError(t, err)
	return u
}
