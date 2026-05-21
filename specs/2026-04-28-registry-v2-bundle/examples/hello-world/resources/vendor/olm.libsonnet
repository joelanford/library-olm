// olm.libsonnet — ClusterObjectSet construction helpers.
// Provided by the OLM maintainers.

function(bundle) {
  metadata:: bundle,

  clusterObjectSet(config, phases, collisionProtection='Prevent')::
    {
      apiVersion: 'orb.operatorframework.io/v1alpha1',
      kind: 'ClusterObjectSet',
      metadata: {
        name: bundle.name,
        labels: {
          'orb.operatorframework.io/owner-name': bundle.name,
        },
      },
      spec: {
        template: {
          spec: {
            collisionProtection: collisionProtection,
            phases: phases,
          },
        },
      },
    },

  phase(name, resourcesFn, config)::
    local results = resourcesFn($.metadata, config);
    {
      name: name,
      objects: [
        { object: r.resource }
        + (if 'assertions' in r then { assertions: r.assertions } else {})
        for r in results
      ],
    },
}
