---
title: Getting Started
weight: -80
---

This page tells you how to get started with the dockhand-lru-registry, including installation and basic usage.

<!--more-->

{{< toc >}}

## Install

`dockhand-lru-registry` is installed via Helm charts:
- dockhand-lru-registry

The chart can be found in the [dockhand-charts](https://github.com/boxboat/dockhand-charts) repository. 

```Shell
# install dockhand-lru-registry
helm repo add dockhand https://dockhand-charts.storage.googleapis.com
helm repo update
helm install --namespace dockhand-lru-registry dockhand/dockhand-lru-registry
```

## Settings

### Helm Chart

#### default `values.yaml` 
```yaml
registry:
  image:
      repository: registry
      pullPolicy: IfNotPresent
      tag: 2.7.1
  conf: |
    version: 0.1
    log:
      fields:
        service: registry
    storage:
      delete:
        enabled: true
      cache:
        blobdescriptor: inmemory
      filesystem:
        rootdirectory: /var/lib/registry
    http:
      addr: :5000
      headers:
        X-Content-Type-Options: [nosniff]
    health:
      storagedriver:
        enabled: true
        interval: 10s
        threshold: 3
  cache:
    enabled: true
    persistence:
      # set to hostPath, emptyDir or pvc
      type: pvc
      pvc:
        #storageClass: "-"
        accessMode: ReadWriteOnce
        size: 100Gi
      hostPath: {}
        # path: /mnt/dir
        # type: DirectoryOrCreate
      emptyDir: {}
        #sizeLimit: 20Gi

proxy:
  debug: false
  port: 3000
  cleanSettings:
    targetDiskUsage: 80Gi
    cleanTagPercentage: 10.0
    cron: "0 2 * * *"
    timezone: "Local"
    # set this to true if the registry isn't sharing a disk
    useSeparateDiskCalculation: false
  image:
    repository: boxboat/dockhand-lru-registry
    pullPolicy: IfNotPresent
    tag: v0.1.0

service:
  type: ClusterIP
  port: 3000

ingress:
  enabled: false
  className: ""
  annotations: {}
    # kubernetes.io/ingress.class: nginx
    # nginx.ingress.kubernetes.io/proxy-body-size: "0"
    # kubernetes.io/tls-acme: "true"
  hosts:
    - host: lru-registry.example.local
      paths:
        - path: /
          pathType: ImplementationSpecific
  tls: []
  #  - secretName: chart-example-tls
  #    hosts:
  #      - chart-example.local

podSecurityContext: {}
  # fsGroup: 2000

securityContext: {}
  # capabilities:
  #   drop:
  #   - ALL
  # readOnlyRootFilesystem: true
  # runAsNonRoot: true
  # runAsUser: 1000

nodeSelector: {}

tolerations: []

affinity: {}
```

#### Considerations
For persistence using a separate dedicated `PersistentVolume` can speed up the maintenance operations by allowing you to set `proxy.cleanSettings.useSeparateDiskCalculation: true`. This will effectively use `statfs` rather than scanning the entire registry directory. Setting `useSeparateDiskCalculation` is not recommended for other storage configurations. 

### Direct Use
```shell
dockhand-lru-registry start --help
start the proxy with the provided settings

Usage:
  dockhand-lru-registry start [flags]

Flags:
      --cert string                   x509 server certificate
      --clean-tags-percentage float   percentage of least recently used tags to remove each iteration of a clean cycle until the target-percentage is achieved (default 10)
      --cleanup-cron string           cron schedule for cleaning up the least recently used tags default is 0:00:00 (default "0 0 * * *")
      --db-dir string                 db directory (default "/var/lib/registry")
  -h, --help                          help for start
      --key string                    x509 server key
      --port int                       (default 3000)
      --registry-bin string           registry binary (default "/registry/bin/registry")
      --registry-conf string          registry config (default "/etc/docker/registry/config.yml")
      --registry-dir string           registry directory (default "/var/lib/registry")
      --registry-host string          registry host (default "127.0.0.1:5000")
      --registry-scheme string        registry scheme (default "http")
      --separate-disk                 registry on separate disk or mount - use optimized disk size calculation
      --target-disk-usage string      target usage of disk for a clean cycle, a scheduled clean cycle will clean tags until this threshold is met (default "50Gi")
      --timezone string               timezone string to use for scheduling based on the cron-string (default "Local")
      --use-forwarded-headers         use x-forwarded headers

Global Flags:
      --config string   config file (default is $HOME/.lru-registry.yaml)
      --debug           debug output
```