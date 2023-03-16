---
title: Dockhand LRU Registry
description: Dockhand Least Recently Used Registry
geekdocNav: false
geekdocAlign: center
geekdocAnchor: false
---

<!-- markdownlint-capture -->
<!-- markdownlint-disable MD033 -->

<span class="badge-placeholder">[![Build Status](https://img.shields.io/github/actions/workflow/status/boxboat/dockhand-lru-registry/docker.yaml?master)](https://github.com/boxboat/dockhand-lru-registry)</span>
<span class="badge-placeholder">[![GitHub release](https://img.shields.io/github/v/release/boxboat/dockhand-lru-registry)](https://github.com/boxboat/dockhand-lru-registry/releases/latest)</span>
<span class="badge-placeholder">[![GitHub contributors](https://img.shields.io/github/contributors/boxboat/dockhand-lru-registry)](https://github.com/boxboat/dockhand-lru-registry/graphs/contributors)</span>
<span class="badge-placeholder">[![License: APACHE](https://img.shields.io/github/license/boxboat/dockhand-lru-registry)](https://github.com/boxboat/dockhand-lru-registry/blob/main/LICENSE)</span>

<!-- markdownlint-restore -->

The `dockhand-lru-registry` is built using Go and acts as a proxy for [distribution/distribution](https://github.com/distribution/distribution). The lru proxy keeps track of image access and has a schedulable maintenance action. During the maintenance window, the `dockhand-lru-registry` will effectively put the registry in readonly mode, and run garbage collection. If target disk usage has exceeded a configurable threshold it will remove a configurable percentage of the least recently used tags from the registry until usage has dropped back below the target threshold.

{{< button size="large" relref="usage/getting-started/" >}}Getting Started{{< /button >}}
