package main

type filesystem string

func (fs filesystem) GetContainerMountFilePath(fsPath, containerHash string) string {
	return fsPath + "/image/overlay2/layerdb/mounts/" + containerHash + "/mount-id"
}

func (fs filesystem) GetParentFileLocation(fsPath, containerHash string) string {
	return fsPath + "/image/overlay2/layerdb/mounts/" + containerHash + "/parent"
}

func (fs filesystem) GetLayerSizePath(fsPath, layerHash string) string {
	return fsPath + "/image/overlay2/layerdb/sha256/" + layerHash + "/size"
}

func (fs filesystem) GetLayerParentPath(fsPath, layerHash string) string {
	return fsPath + "/image/overlay2/layerdb/sha256/" + layerHash + "/parent"
}

func (fs filesystem) GetCacheIDPath(fsPath, layerHash string) string {
	return fsPath + "/image/overlay2/layerdb/sha256/" + layerHash + "/cache-id"
}

func (fs filesystem) GetMntPath(fsPath, layerHash string) string {
	return fsPath + "/overlay2/mnt/" + layerHash + "/"
}

// Filesystem exports the current fs data structure
var Filesystem filesystem
