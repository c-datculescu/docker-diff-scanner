package main

type filesystem string

func (fs filesystem) GetContainerMountPath(fsPath, containerHash string) string {
	return fsPath + "/image/devicemapper/layerdb/mounts/" + containerHash + "/mount-id"
}

func (fs filesystem) GetParentFileLocation(fsPath, containerHash string) string {
	return fsPath + "/image/devicemapper/layerdb/mounts/" + containerHash + "/parent"
}

func (fs filesystem) GetLayerSizePath(fsPath, layerHash string) string {
	return fsPath + "/image/devicemapper/layerdb/sha256/" + layerHash + "/size"
}

func (fs filesystem) GetLayerParentPath(fsPath, layerHash string) string {
	return fsPath + "/image/devicemapper/layerdb/sha256/" + layerHash + "/parent"
}

// Filesystem exports the current fs data structure
var Filesystem filesystem
