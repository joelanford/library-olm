function(bundle, config)
  local olm = (import 'vendor/olm.libsonnet')(bundle);
  local cfg = std.mergePatch((import 'lib/defaults.libsonnet')(bundle), config);

  olm.clusterObjectSet(cfg, [
    olm.phase('namespace', import 'lib/namespace.libsonnet', cfg),
    olm.phase('policy', import 'lib/policy.libsonnet', cfg),
    olm.phase('crds', import 'lib/crds.libsonnet', cfg),
    olm.phase('rbac', import 'lib/rbac.libsonnet', cfg),
    olm.phase('deploy', import 'lib/deploy.libsonnet', cfg),
  ])
