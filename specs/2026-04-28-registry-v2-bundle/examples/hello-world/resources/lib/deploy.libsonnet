local k8s = import '../vendor/k8s-common.libsonnet';

function(bundle, config)
  local labels = k8s.labels(bundle.name, bundle.version, bundle.release);
  [
    {
      assertions: k8s.assertions.deploymentReady,
      resource: k8s.deployment.operator(
        name='widget-operator',
        namespace=config.namespace,
        image=config.image,
        labels=labels { 'app.kubernetes.io/component': 'operator' },
        replicas=config.replicas,
      ) {
        spec+: { template+: { spec+: {
          containers: [
            super.containers[0] {
              volumeMounts: [{ name: 'tmp', mountPath: '/tmp' }],
            },
          ],
          volumes: [{ name: 'tmp', emptyDir: {} }],
        } } },
      },
    },
    {
      resource: k8s.service.metrics(
        name='widget-operator',
        namespace=config.namespace,
        labels=labels { 'app.kubernetes.io/component': 'metrics' },
      ),
    },
  ]
