local k8s = import '../vendor/k8s-common.libsonnet';

function(bundle, config)
  k8s.rbac.managerBinding('widget-operator', config.namespace, [
    { group: 'example.com', resource: 'widgets' },
  ])
