package main

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os/exec"

	stackerrors "github.com/go-errors/errors"
)

// Container represents a container reported by docker
type Container struct {
	Name             string
	Hash             string
	COWSize          int64                // the current size of the filesystem allocated to the current container
	COWLocation      string               // the path for the current filesystem allocated to the container
	ParentChain      *ContainerLayer      // the parent chain for the current container
	ContainerDetails *DockerInspectResult // contains information about the current running docker container

	Filesystem FilesystemPather // the path finder for the current filesystem
}

// Init performs all the needed functions for populating the current container
func (c *Container) Init() error {
	err := c.GetContainerDetails()
	if err != nil {
		return err
	}

	err = c.GetCOWSize()
	if err != nil {
		return err
	}

	err = c.GetParentLayer()
	if err != nil {
		return err
	}

	return nil
}

// GetContainerDetails returns details about the currently monitored container
func (c *Container) GetContainerDetails() error {
	output, err := exec.Command("docker", "inspect", c.Hash).Output()
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}

	// decode the json
	var decoded []*DockerInspectResult
	err = json.Unmarshal(output, &decoded)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}
	if len(decoded) == 0 {
		return errors.New("No containers matching hash: " + c.Hash + " exist!")
	}

	c.ContainerDetails = decoded[0]
	return nil
}

// GetCOWSize returns the size of the filesystem occupied by the current container
func (c *Container) GetCOWSize() error {
	// this file will indicate where to look for the mount location, but it is not the actual mount location
	mountFileLocation := c.Filesystem.GetContainerMountFilePath(*fsPath, c.Hash)
	contents, err := ioutil.ReadFile(mountFileLocation)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}
	// this will return the proper mount location on which we can actually calculate the size of the filesystem for the current container
	mountLocation := c.Filesystem.GetMntPath(*fsPath, string(contents))
	// read the file and get the proper
	size, err := CalculateFolderSize(mountLocation)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}

	// allocate the filesystem size and path for easy processing
	c.COWSize = size
	c.COWLocation = mountLocation
	return nil
}

// GetParentLayer returns a layer that represents the parent of the current container
func (c *Container) GetParentLayer() error {
	parentFileLocation := c.Filesystem.GetParentFileLocation(*fsPath, c.Hash)
	// read the contents of the file
	contents, err := ioutil.ReadFile(parentFileLocation)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}

	// initialize the layer chain
	c.ParentChain = NewContainerLayer(string(contents), c.Filesystem, c)
	err = c.ParentChain.Init()
	if err != nil {
		return err
	}

	return nil
}
