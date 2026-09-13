package catalogv1

import "errors"

// ErrNotFound classifies the absence of a requested catalog resource.
// Optional properties do not use this error when they are absent.
var ErrNotFound = errors.New("catalog resource not found")
