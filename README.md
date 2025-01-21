# dockhand-lru-registry
`dockhand-lru-registry` acts as proxy to [Distribution](https://github.com/distribution/distribution) to make a registry with a 
cleanup policy based on least recently used tags. The registry keeps track of push/pull operations, monitors disk 
utilization and runs the registry garbage collector on a configurable schedule. When the registry exceeds the target disk 
utilization, a configurable percentage of the least recently used tags will be removed until disk utilization drops back
below the target. 

## Garbage Collection
Garbage Collection can be scheduled and will turn the registry into read only mode via the proxy by only handling pulls 
while garbage collection is occurring.

## Usage
See [charts](./charts/dockhand-lru-registry) for Kubernetes installation. 

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