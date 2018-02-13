package main

import (
	"errors"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	stackerrors "github.com/go-errors/errors"
)

// NewContainerLayer returns a new fresh container layer
// Attention: the function can receive a nil container, as it is not needed to pass a container to all the layers in the chain
// Just the first layer (parent layer of a container) will contain the list of containers allocated, since all the lower level layers
// will just be inherited for all containers
func NewContainerLayer(sha256hash string, filesystem FilesystemPather, container *Container) *ContainerLayer {
	// check if the hash is in the format: sha256:hash
	bits := strings.Split(sha256hash, ":")
	if len(bits) == 2 {
		sha256hash = bits[1]
	}

	// check if the containerLayer already exists, increase the counters and allocate the containers (if we have them)
	if value, exists := ExistingLayers[sha256hash]; exists == true {
		value.SharedCount++
		if container == nil {
			return value
		}
		value.Containers = append(value.Containers, container.ContainerDetails.Name)
		return value
	}

	// initialize a new layer, allocate the current container and set the shared counter for it
	layer := &ContainerLayer{
		Hash:        sha256hash,
		Filesystem:  filesystem,
		SharedCount: 1,
	}
	if container != nil {
		layer.Containers = append(layer.Containers, container.ContainerDetails.Name)
	}

	ExistingLayers[sha256hash] = layer

	return layer
}

// ExistingLayers records the existing layers so we do not have to calculate them again
var ExistingLayers = map[string]*ContainerLayer{}

// ContainerLayer represents a layer that the container uses
// Various layers will be shared between various containers, these
// statistics will also be presented
type ContainerLayer struct {
	Size     int64 // currently recorded size for the layer
	Hash     string
	Location string
	Parent   *ContainerLayer

	SharedCount int      // the number of times this layer is shared across containers
	Containers  []string // the list of containers that share this layer

	Filesystem FilesystemPather
}

// Init initializes the current layer and calculates
// parents
// size
func (c *ContainerLayer) Init() error {
	cacheIDLocation := c.Filesystem.GetCacheIDPath(*fsPath, c.Hash)
	contents, err := ioutil.ReadFile(cacheIDLocation)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}
	cacheID := string(contents)
	bits := strings.Split(string(contents), ":")
	if len(bits) == 2 {
		cacheID = bits[1]
	}
	c.Location = c.Filesystem.GetMntPath(*fsPath, cacheID)

	if c.Parent != nil {
		// the parent has already initialized. no need to do anthing there
		return nil
	}
	err = c.ReadSize()
	if err != nil {
		return err
	}
	c.GetParent()
	return nil
}

// ReadSize reads the size of the current layer as recorded in the filesystem
func (c *ContainerLayer) ReadSize() error {
	sizeLocation := c.Filesystem.GetLayerSizePath(*fsPath, c.Hash)
	contents, err := ioutil.ReadFile(sizeLocation)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}

	if string(contents) == "" {
		c.Size = -100
	}

	size, err := strconv.ParseInt(string(contents), 10, 64)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}

	c.Size = size

	return nil
}

// GetParent allocates the parent of the current layer if it exists. In the case this is a final layer,
// this will be nil
func (c *ContainerLayer) GetParent() error {
	parentLocation := c.Filesystem.GetLayerParentPath(*fsPath, c.Hash)

	// check if file exists on disk.
	// If there is no file existing on disk, this is the last layer, so the parent will be nil
	if _, err := os.Stat(parentLocation); os.IsNotExist(err) {
		c.Parent = nil
		return nil
	}

	contents, err := ioutil.ReadFile(parentLocation)
	if err != nil {
		return stackerrors.Wrap(err, 1)
	}

	// get the sha of the hash. The sha will be in the format: sha256:hash
	bits := strings.Split(string(contents), ":")
	var parentHash string
	if len(bits) == 2 {
		parentHash = bits[1]
	} else {
		return errors.New("The hash is wrong: " + string(contents))
	}

	c.Parent = NewContainerLayer(parentHash, c.Filesystem, nil)
	err = c.Parent.Init()
	if err != nil {
		return nil
	}

	return nil
}
