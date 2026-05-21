// k8s-common.libsonnet — common Kubernetes resource helpers.
{
  assertions:: {
    crdEstablished:: [
      { conditionEqual: { type: 'Established', status: 'True' } },
    ],

    deploymentReady:: [
      { conditionEqual: { type: 'Available', status: 'True' } },
      { fieldsEqual: { fieldA: '.spec.replicas', fieldB: '.status.updatedReplicas' } },
      { fieldsEqual: { fieldA: '.spec.replicas', fieldB: '.status.readyReplicas' } },
      { fieldsEqual: { fieldA: '.spec.replicas', fieldB: '.status.replicas' } },
    ],
  },

  labels(name, version, release='', component=null):: {
    'app.kubernetes.io/name': name,
    'app.kubernetes.io/version': version,
    'app.kubernetes.io/managed-by': 'olm',
    [if release != '' then 'app.kubernetes.io/release']: release,
    [if component != null then 'app.kubernetes.io/component']: component,
  },

  rbac:: {
    rule(apiGroups, resources, verbs):: {
      apiGroups: apiGroups,
      resources: resources,
      verbs: verbs,
    },

    managerRules(managed)::
      [
        self.rule([m.group], [m.resource], ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete'])
        for m in managed
      ] +
      [
        self.rule([m.group], [m.resource + '/status'], ['get', 'update', 'patch'])
        for m in managed
      ] +
      [
        self.rule([''], ['events'], ['create', 'patch']),
      ],

    serviceAccount(name, namespace):: {
      apiVersion: 'v1',
      kind: 'ServiceAccount',
      metadata: { name: name, namespace: namespace },
    },

    clusterRole(name, rules):: {
      apiVersion: 'rbac.authorization.k8s.io/v1',
      kind: 'ClusterRole',
      metadata: { name: name },
      rules: rules,
    },

    clusterRoleBinding(name, roleName, saName, saNamespace):: {
      apiVersion: 'rbac.authorization.k8s.io/v1',
      kind: 'ClusterRoleBinding',
      metadata: { name: name },
      roleRef: {
        apiGroup: 'rbac.authorization.k8s.io',
        kind: 'ClusterRole',
        name: roleName,
      },
      subjects: [{
        kind: 'ServiceAccount',
        name: saName,
        namespace: saNamespace,
      }],
    },

    managerBinding(name, namespace, managed)::
      local rbac = self;
      [
        { resource: rbac.serviceAccount(name, namespace) },
        { resource: rbac.clusterRole(name, rbac.managerRules(managed)) },
        { resource: rbac.clusterRoleBinding(name, name, name, namespace) },
      ],
  },

  deployment:: {
    operator(name, namespace, image, labels={}, replicas=1, port=8080):: {
      local selectorLabels = { app: name },
      apiVersion: 'apps/v1',
      kind: 'Deployment',
      metadata: {
        name: name,
        namespace: namespace,
        labels: labels + selectorLabels,
      },
      spec: {
        replicas: replicas,
        selector: { matchLabels: selectorLabels },
        template: {
          metadata: { labels: labels + selectorLabels },
          spec: {
            serviceAccountName: name,
            securityContext: {
              runAsNonRoot: true,
              seccompProfile: { type: 'RuntimeDefault' },
            },
            containers: [{
              name: 'operator',
              image: image,
              ports: [{ containerPort: port, name: 'metrics' }],
              securityContext: {
                allowPrivilegeEscalation: false,
                readOnlyRootFilesystem: true,
                capabilities: { drop: ['ALL'] },
              },
              livenessProbe: {
                httpGet: { path: '/', port: port },
                initialDelaySeconds: 5,
                periodSeconds: 10,
              },
              readinessProbe: {
                httpGet: { path: '/', port: port },
                initialDelaySeconds: 2,
                periodSeconds: 5,
              },
            }],
          },
        },
      },
    },
  },

  service:: {
    metrics(name, namespace, labels={}, port=8080):: {
      apiVersion: 'v1',
      kind: 'Service',
      metadata: {
        name: name + '-metrics',
        namespace: namespace,
        labels: labels,
      },
      spec: {
        selector: { app: name },
        ports: [{ port: port, targetPort: 'metrics', protocol: 'TCP', name: 'metrics' }],
      },
    },
  },
}
