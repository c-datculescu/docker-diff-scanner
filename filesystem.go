package main

import (
	"flag"
	"fmt"
	"log"
	"time"
)

// FilesystemPather is the interface responsible for returning paths for each supported filesystem
type FilesystemPather interface {
	GetContainerMountFilePath(fsPath, containerHash string) string
	GetParentFileLocation(fsPath, containerHash string) string
	GetLayerSizePath(fsPath, layerHash string) string
	GetLayerParentPath(fsPath, layerHash string) string
	GetCacheIDPath(fsPath, layerHash string) string
	GetMntPath(fsPath, layerHash string) string
}

// DockerInspectResult is the result of inspecting the needed container
type DockerInspectResult struct {
	Name         string                    `json:"Name"`         // the name of the current container prefixed with /
	State        *DockerInspectStateResult `json:"State"`        // the state of the current container
	RestartCount int                       `json:"RestartCount"` // the number of executed restarts so far
}

// DockerInspectStateResult is the result for the state subrecord
type DockerInspectStateResult struct {
	Status    string    `json:"Status"`    // the current status of the container as a string
	Pid       int       `json:"Pid"`       // the pid of the current container
	StartedAt time.Time `json:"StartedAt"` // the time that the container was started
}

var (
	fsPath = flag.String("fs-path", "/var/lib/docker", "The path where the docker filesystem can be located")
	fs     = flag.String("fs", "aufs", "The filesystem current docker daemon uses")
)

func main() {
	flag.Parse()
	filesystem := loadFilesystemPlugin(*fs)

	containers, err := GetAllContainers(filesystem)
	if err != nil {
		log.Fatal(err)
	}
	for _, container := range containers {
		// start printing the details about the current container
		fmt.Printf(
			"COWLocation: %s\nCOWSize: %d\nName: %s\nHash: %s\nStatus: %s\nStartedAt: %s\n\nParents:\n",
			container.COWLocation,
			container.COWSize,
			container.ContainerDetails.Name,
			container.Hash,
			container.ContainerDetails.State.Status,
			container.ContainerDetails.State.StartedAt,
		)
		RecursivePrintParents(container.ParentChain)
	}
}
