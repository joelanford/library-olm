# Implementation Plan

1. Add `Validate` function to `bundle/registry/v1/registryv1.go` that delegates
   to `registryv1.BundleValidator.Validate(&b)`.
