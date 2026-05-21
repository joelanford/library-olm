local k8s = import '../vendor/k8s-common.libsonnet';

function(bundle, config) [
  {
    assertions: k8s.assertions.crdEstablished,
    resource: std.parseYaml(importstr '../static/widgets.crd.yaml'),
  },
]
