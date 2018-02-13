package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"plugin"
	"strconv"
	"strings"
	"time"
)

// FilesystemPather is the interface responsible for returning paths for each supported filesystem
type FilesystemPather interface {
	GetContainerMountPath(fsPath, containerHash string) string
	GetParentFileLocation(fsPath, containerHash string) string
	GetLayerSizePath(fsPath, layerHash string) string
	GetLayerParentPath(fsPath, layerHash string) string
	GetLayerPath(fsPath, layerHash string) string
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
		return err
	}

	// decode the json
	var decoded []*DockerInspectResult
	err = json.Unmarshal(output, &decoded)
	if err != nil {
		return err
	}
	if len(decoded) == 0 {
		return errors.New("No containers matching hash: " + c.Hash + " exist!")
	}

	c.ContainerDetails = decoded[0]
	return nil
}

// GetCOWSize returns the size of the filesystem occupied by the current container
func (c *Container) GetCOWSize() error {
	mountLocation := c.Filesystem.GetContainerMountPath(*fsPath, c.Hash)
	size, err := CalculateFolderSize(mountLocation)
	if err != nil {
		return err
	}

	// allocate the COW size and path for easy processing
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
		return err
	}

	c.ParentChain = NewContainerLayer(string(contents), c.Filesystem, c)
	// initialize the layer chain
	err = c.ParentChain.Init()
	if err != nil {
		return err
	}

	return nil
}

// NewContainerLayer returns a new fresh container layer
func NewContainerLayer(sha256hash string, filesystem FilesystemPather, container *Container) *ContainerLayer {
	// check if the hash is in the format: sha256:hash
	bits := strings.Split(sha256hash, ":")
	if len(bits) == 2 {
		sha256hash = bits[1]
	}

	// check if the containerLayer already exists and return it without actually doing anything
	if value, exists := ExistingLayers[sha256hash]; exists == true {
		value.SharedCount++
		if container == nil {
			return value
		}
		value.Containers = append(value.Containers, container.ContainerDetails.Name)
		return value
	}
	layer := &ContainerLayer{
		Hash:       sha256hash,
		Filesystem: filesystem,
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
	c.Location = c.Filesystem.GetLayerPath(*fsPath, c.Hash)
	if c.Parent != nil {
		// the parent has already initialized. no need to do anthing there
		return nil
	}
	err := c.ReadSize()
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
		return err
	}

	size, err := strconv.ParseInt(string(contents), 10, 64)
	if err != nil {
		return err
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
		return err
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

// loadFilesystemPlugin loads the apropriate plugin representing the filesystem. The plugin will be responsible for
// returning the paths where we can find the apropriate folders containing the information
func loadFilesystemPlugin(filesystem string) FilesystemPather {
	// check which filesystem plugin do we need to load
	// issue error and stop if the filesystem is not available for load
	var mod string
	switch filesystem {
	case "aufs":
		mod = "./plugins/aufs/aufs.so"
	case "overlay2":
		mod = "./plugins/overlay2/overlay2.so"
	case "devicemapper":
		mod = "./plugins/devicemapper/devicemapper.so"
	default:
		log.Fatal(errors.New("Supported filesystems: aufs, overlay2, devicemapper. Provided filesystem " + filesystem + " is currently not supported!"))
	}

	// attempt to open the plugin. Issue error if the plugin cannot be loaded for whatever reason
	filesystemPlugin, err := plugin.Open(mod)
	if err != nil {
		log.Fatal(err)
	}

	// lookup the needed data structures that we require in order to make sure we have the correct
	// plugin loaded
	loadedFS, err := filesystemPlugin.Lookup("Filesystem")
	if err != nil {
		log.Fatal(err)
	}

	// check te type of the loaded plugin, to make sure it is compatible with what we need
	pluginFilesystem, ok := loadedFS.(FilesystemPather)
	if !ok {
		log.Fatal(errors.New("Cannot load symbol, the typecheck failed"))
	}

	return pluginFilesystem
}

// CalculateFolderSize calculates the folder size given a path
func CalculateFolderSize(folderPath string) (int64, error) {
	diskCommand := exec.Command("du", "-sb", folderPath)
	getFirstColumnCommand := exec.Command("cut", "-f1")
	reader, writer := io.Pipe()
	diskCommand.Stdout = writer
	getFirstColumnCommand.Stdin = reader

	var outputBuffer bytes.Buffer
	getFirstColumnCommand.Stdout = &outputBuffer

	diskCommand.Start()
	getFirstColumnCommand.Start()
	diskCommand.Wait()
	writer.Close()
	getFirstColumnCommand.Wait()
	// read the full contents of the buffer
	trimmed := string(strings.TrimSpace(outputBuffer.String()))

	size, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil {
		return 0, err
	}

	return size, nil
}

// GetAllContainers returns all the containers currently running in the target server
func GetAllContainers(filesystemPlugin FilesystemPather) ([]*Container, error) {
	files, err := ioutil.ReadDir(*fsPath + "/containers/")
	if err != nil {
		return nil, err
	}

	containers := []*Container{}
	for _, file := range files {
		container := &Container{
			Hash:       file.Name(),
			Filesystem: filesystemPlugin,
		}
		err := container.Init()
		if err != nil {
			return nil, err
		}
		containers = append(containers, container)
	}

	return containers, nil
}

// RecursivePrintParents takes a parent and recursively prints the layers it uses
func RecursivePrintParents(layer *ContainerLayer) {
	format := "\tLocation: %s\n\tSize: %d\n\tHash: %s\n\tShared: %d\n\tContainers: %v\n\t===\n"
	if layer != nil {
		fmt.Printf(
			format,
			layer.Location,
			layer.Size,
			layer.Hash,
			layer.SharedCount,
			layer.Containers,
		)
		RecursivePrintParents(layer.Parent)
	} else {
		fmt.Println("\n==========")
	}
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
			"Name: %s\nHash: %s\nStatus: %s\nStartedAt: %s\n\nParents:\n",
			container.ContainerDetails.Name,
			container.Hash,
			container.ContainerDetails.State.Status,
			container.ContainerDetails.State.StartedAt,
		)
		RecursivePrintParents(container.ParentChain)
	}
}
