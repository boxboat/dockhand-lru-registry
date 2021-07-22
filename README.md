# dockhand-lru-registry
`dockhand-lru-registry` acts as proxy to [Distribution](github.com/distribution/distribution) to make a registry with a cleanup policy based on least recently used tags.

## Garbage Collection
Garbage Collection can be scheduled and will turn the registry into read only mode via the proxy by only handling pulls 
while garbage collection is occurring.