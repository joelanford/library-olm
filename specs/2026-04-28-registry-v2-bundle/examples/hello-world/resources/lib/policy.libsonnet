function(bundle, config) [
  {
    resource: {
      apiVersion: 'admissionregistration.k8s.io/v1',
      kind: 'ValidatingAdmissionPolicy',
      metadata: { name: 'widget-validation' },
      spec: {
        failurePolicy: 'Fail',
        matchConstraints: {
          resourceRules: [{
            apiGroups: ['example.com'],
            apiVersions: ['v1'],
            operations: ['CREATE', 'UPDATE'],
            resources: ['widgets'],
          }],
        },
        validations: [
          {
            expression: "object.spec.size in ['small', 'medium', 'large']",
            message: 'size must be small, medium, or large',
          },
          {
            expression: 'object.metadata.name.size() <= 63',
            message: 'widget name must be 63 characters or fewer',
          },
        ],
      },
    },
  },
  {
    resource: {
      apiVersion: 'admissionregistration.k8s.io/v1',
      kind: 'ValidatingAdmissionPolicyBinding',
      metadata: { name: 'widget-validation' },
      spec: {
        policyName: 'widget-validation',
        validationActions: ['Deny'],
      },
    },
  },
]
