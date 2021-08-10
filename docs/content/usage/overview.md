---
title: Overview
weight: -100
---

## What is the Dockhand LRU Registry?

Dockhand LRU Registry is a proxy that simplifies maintenance of Docker registry by garbage collecting based on a least recently used policy.

## Why Dockhand LRU Registry?
The main use case for this registry is as a build image cache. The trend of building Docker images on Kubernetes clusters necessitates a build cache to ensure optimal build times. In most case you don't want to use your production image registry for this purpose and ideally you want something that requires very little manual intervention. The `dockhand-lru-registry` solves this problem by tracking image tag usage, performing scheduled garbage collection and when necessary will remove the oldest tags first to ensure size constraints are met.

## How it works
`dockhand-lru-registry` acts a proxy for a vanilla open source [Docker Registry](https://github.com/distribution/distribution). As requests push and pull pass through the proxy it updates access time for the image being accessed. Maintenance is schedulable via a familar cron string format. When maintenance is occuring, the proxy will put the registry in readonly mode, run garbage collection, and if necessary remove a percentage of tags to drop the disk usage below the desired threshold.
