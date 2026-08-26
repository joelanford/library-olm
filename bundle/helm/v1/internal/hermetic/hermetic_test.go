package hermetic

import (
	"testing"

	"github.com/Masterminds/sprig/v3"
	"github.com/stretchr/testify/require"
)

func TestOverrides(t *testing.T) {
	tests := []struct {
		name         string
		functionName string
		shouldError  bool
	}{
		{"allowed function is not overridden", "upper", false},
		{"lookup is unsupported", "lookup", true},
		{"non-allowlisted function is unsupported", "randAlphaNum", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, isOverridden := Overrides()[tc.functionName]
			require.Equal(t, tc.shouldError, isOverridden)

			if tc.shouldError {
				errFunc, ok := f.(func(...any) (any, error))
				require.True(t, ok)
				_, err := errFunc()
				target := &UnsupportedTemplateFunction{}
				require.ErrorAs(t, err, &target)
				require.Contains(t, err.Error(), tc.functionName)
			}

		})
	}
}

func TestOverridesCoverSprigFuncMap(t *testing.T) {
	overrides := Overrides()
	for name := range sprig.TxtFuncMap() {
		_, isAllowed := allowed[name]
		_, isDenied := overrides[name]
		require.NotEqual(t, isAllowed, isDenied, "function %q must be allowed or denied", name)
	}
	for name := range allowed {
		_, exists := sprig.TxtFuncMap()[name]
		require.True(t, exists, "allowlist contains unknown Sprig function %q", name)
	}
}
