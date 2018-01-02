# Docker-filesystem

This is a small management utility that identifies potential issues
with the docker filesystem and can allow certain operations that might
aleviate especially the out of disk or slowly leaking disk situation.

## How does it work

The tool scans the docker filesystem and builds the graph which each container
is using. Generally the biggest issue is that the diff folders become too 
large, leading to disk space exhaustion.

Sometimes it can be tricky to identify what is causing the issue, so this tool
attempts to provide a more comprehensive view over the entire filesystem.

## Docker filesystem

Regardless of the flesystem used, docker provides the same basic structure for
the filesystem, located in `/var/lib/docker` usually (for non-custom installations).

The important paths are listed below:

- `/var/lib/docker/image` - the place where all the magic happens
- `/var/lib/docker/{container full id}/` - a running or stopped container
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/` - the layer structure for the existing containers (in whatever state)
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/mounts` - the mounts definition for all the containers
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/mounts/mount-id` - the mount of the current container being investigated
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/mounts/init-id` - init mount of the current container being investigated
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/mounts/parent` - the sha256 hash of the parent layer
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/sha256` - the place where all the layers belonging to containers are located
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/sha256/{sha256 hash}/cache-id` - the cache identifier for the layer which hash is {sha256 hash}
- `/var/lib/docker/{container full id}/{filesystem}/layerdb/{sha256 hash}/parent` - the direct parent of the current layer with the hash {sha256 hash}. __If this file is missing, this ia the lowest layer possible, so it has no parent.__

The files:

- `mount-id`
- `init-id`
- `cache-id`

correspond to additional folders under `/var/lib/docker/{filesystem}/{specified hash}`

Based on the presente structure, we can generate a map of all the layers being used by
containers in various states, allowing us to determine which data layers are not actually
used and try to remove them if needed.

## Additional features

Additional features include

- analysis of layers forming a container and potential optimizations

For example, if an running container is made out four layers, if in each layer we do
a chown -R www-data. /, the initial size of the first layer will be quadruplicated because
of how union filesystems function (copy on write).