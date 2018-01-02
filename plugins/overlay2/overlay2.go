// the overlay2 directory structure is diferend than the one provided by aufs
// so we require differend approaches to process various types of storage systems
package main

type filesystem string

// GetContainerFolderPath returns the path that corresponds to the folder where all containers
// are located
func (fs filesystem) GetContainerFolderPath(basePath string) string {
	return basePath + "/image/overlay2/layerdb/mounts/"
}

// GetImagePath returns the path that corresponds to the location of the image folder
// aka. /var/lib/docker/image/overlay2/
func (fs filesystem) GetContainerInitFilePath(basePath, containerFullHash string) string {
	return basePath + "/image/overlay2/layerdb/mounts/" + containerFullHash + "/init-id"
}

// GetMountFilePath returns the path for the mount-id file in the docker filesystem
// this holds a reference to the location of the actual data file
func (fs filesystem) GetContainerMountFilePath(basePath, containerFullHash string) string {
	return basePath + "/image/overlay2/layerdb/mounts/" + containerFullHash + "/mount-id"
}

// GetDiffPath returns the diff path for a provided hash
func (fs filesystem) GetDiffFolderPath(basePath, layerHash string) string {
	return basePath + "/overlay2/" + layerHash + "/diff/"
}

// GetCachePath returns the cache-id file path for the current given hash
func (fs filesystem) GetLayerCacheFilePath(basePath, sha256Hash string) string {
	return basePath + "/image/overlay2/layerdb/sha256/" + sha256Hash + "/cache-id"
}

// GetContainerParentPath returns the path where we can find the parent file for the given container
func (fs filesystem) GetContainerParentPath(basePath, containerFullHash string) string {
	return basePath + "/image/overlay2/layerdb/mounts/" + containerFullHash + "/parent"
}

// GetLayerParentPath returns the path where we can find the parent of the given layer
func (fs filesystem) GetLayerParentPath(basePath, sha256Hash string) string {
	return basePath + "/image/overlay2/layerdb/sha256/" + sha256Hash + "/parent"
}

// GetDiffPath returns the path where all the diffs are present
func (fs filesystem) GetDiffPath(basePath string) string {
	return basePath + "/overlay2/"
}

// ProcessDiffFolders eliminates from the list of folders fed the ones that do not
// belong directly to a diff
func (fs filesystem) ProcessDiffFolders(folders []string) []string {
	retDiffs := []string{}
	for _, folder := range folders {
		if folder == "." || folder == ".." || folder == "l" || folder == "" {
			continue
		}
		retDiffs = append(retDiffs, folder)
	}

	return retDiffs
}

// Filesystem exports the current fs data structure
var Filesystem filesystem
