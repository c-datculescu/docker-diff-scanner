package main

import (
	"bytes"
	"container/list"
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
)

// Filesystem represents a particular implementation for a filesystem coming in from separate plugins
type Filesystem interface {
	GetContainerFolderPath(basePath string) string
	GetContainerInitFilePath(basePath, containerFullHash string) string
	GetContainerMountFilePath(basePath, containerFullHash string) string
	GetContainerParentPath(basePath, containerFullHash string) string
	GetDiffFolderPath(basePath, layerHash string) string
	GetLayerCacheFilePath(basePath, sha256Hash string) string
	GetLayerParentPath(basePath, sha256Hash string) string
	GetDiffPath(basePath string) string
	ProcessDiffFolders(initialFolders []string) []string
}

var (
	filesystem       = flag.String("fs", "aufs", "The current storage filesystem for docker")
	dockerLibPath    = flag.String("lib-path", "/var/lib/docker/", "The path to the docker managed files")
	loadedFilesystem Filesystem
)

// retrieves all the current containers
// return error if something goes wrong
func getCurrentContainers() ([]string, error) {
	output, err := exec.Command("ls", loadedFilesystem.GetContainerFolderPath(*dockerLibPath)).Output()
	if err != nil {
		return nil, err
	}

	// clean the output. We do not need the empty lines, folders: . and ..
	tempContainers := strings.Split(string(output), "\n")
	containers := []string{}
	for _, container := range tempContainers {
		if container != "." && container != ".." && container != "" {
			containers = append(containers, container)
		}
	}

	return containers, nil
}

// getCurrentDiffFolders returns the diff related folders currently recorded by docker
func getCurrentDiffFolders() []string {
	diffPath := loadedFilesystem.GetDiffPath(*dockerLibPath)
	output, err := exec.Command("ls", diffPath).Output()
	if err != nil {
		log.Fatal(err)
	}
	folders := strings.Split(string(output), "\n")
	finalDiffFolders := loadedFilesystem.ProcessDiffFolders(folders)
	return finalDiffFolders
}

// loadFilesystemPlugin loads a dynamic plugin based on the filesystem that
// is provided as a value
func loadFilesystemPlugin() {
	// check which filesystem plugin do we need to load
	// issue error and stop if the filesystem is not available for load
	var mod string
	switch *filesystem {
	case "aufs":
		mod = "./plugins/aufs/aufs.so"
	case "overlay2":
		mod = "./plugins/overlay2/overlay2.so"
	default:
		log.Fatal(errors.New("Supported filesystems: aufs, overlay2. Provided filesystem " + *filesystem + " is currently not supported!"))
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
	pluginFilesystem, ok := loadedFS.(Filesystem)
	if !ok {
		log.Fatal(errors.New("Cannot load symbol, the typecheck failed"))
	}

	// allocate the current plugin in order to be used
	loadedFilesystem = pluginFilesystem
}

func main() {
	// process the flags
	flag.Parse()

	// load the plugin for the current filesystem
	loadFilesystemPlugin()

	// get the current containers (running, stopped, failed, etc.)
	containers, err := getCurrentContainers()
	if err != nil {
		log.Println("Error processing containers: " + err.Error())
		os.Exit(2)
	}

	allContainers := map[string]*Container{}

	// process containers
	for _, container := range containers {
		localContainer := &Container{
			ContainerData: &ContainerData{
				ID: container,
			},
		}

		allContainers[container] = localContainer
		localContainer.init()
	}

	// get the sumary:
	for containerHash, container := range allContainers {
		fmt.Printf("Statistics for continer: %s\n", containerHash)
		fmt.Printf("- %d layers\n", container.Parents.Len())
		fmt.Printf("- init diff location: %s\n", container.InitID)
		fmt.Printf("- init diff size: %d\n", container.InitDiffSize)
		fmt.Printf("- mount diff location: %s\n", container.MountID)
		fmt.Printf("- mount diff size: %d\n", container.MountDiffSize)
		fmt.Printf("Layers statistics:\n")
		for e := container.Parents.Front(); e != nil; e = e.Next() {
			fmt.Printf("\tLayer id: %s\n", e.Value.(ContainerParent).Hash)
			fmt.Printf("\t- cache diff location: %s\n", e.Value.(ContainerParent).CacheID)
			fmt.Printf("\t- cache diff size: %d\n", e.Value.(ContainerParent).CacheDiffSize)
		}
	}

	diffFolders := getCurrentDiffFolders()
	hasUnusedLayers := false
	// check if we have layers that are not used
	for _, folder := range diffFolders {
		found := false
		for _, container := range allContainers {
			if folder == container.InitID {
				found = true
				break
			}

			if folder == container.MountID {
				found = true
				break
			}

			for e := container.Parents.Front(); e != nil; e = e.Next() {
				if e.Value.(ContainerParent).CacheID == folder {
					found = true
					break
				}
			}
		}
		if found == false {
			fmt.Printf("Diff %s is orphaned!\n", folder)
			hasUnusedLayers = true
		}
	}

	if hasUnusedLayers == false {
		fmt.Println("No unused layers detected!")
	}
}

// Container represents the full data associated with a container
type Container struct {
	ContainerData *ContainerData // basic data associated with a container
	InitID        string         // the init data layer.
	MountID       string         // the mount id of the container data layer
	Parents       *list.List     // the list of container parents available along with some info about them
	InitDiffSize  int64          // the size of the diff for the current container init layer
	MountDiffSize int64          // the size of the diff folder allocated to the current container mount
}

// init initializes the container, populating all the fields with the provided values
func (c *Container) init() error {
	if err := c.retrieveInitID(); err != nil {
		return err
	}
	if err := c.retrieveMountID(); err != nil {
		return err
	}
	if err := c.calculateMountDiffSize(); err != nil {
		return err
	}
	if err := c.calcaulateInitDiffSize(); err != nil {
		return err
	}
	return c.retrieveParents()
}

// retrieves all the parents of the current container and stores them
func (c *Container) retrieveParents() error {
	contents, err := readFileData(loadedFilesystem.GetContainerParentPath(*dockerLibPath, c.ContainerData.ID))
	if err != nil {
		return err
	}
	parentHash := getHashFromSha256(contents)
	// create a new parent struct and allocate the needed fields inside
	parent := &ContainerParent{
		AssignedContainer: c,
		Hash:              parentHash,
	}
	c.Parents = list.New()
	c.Parents.PushBack(parent)
	parent.init()

	return nil
}

// calculates the diff size for the current layer of the current container
func (c *Container) calcaulateInitDiffSize() error {
	size, err := calculateFolderSize(loadedFilesystem.GetDiffFolderPath(*dockerLibPath, c.InitID))
	if err != nil {
		return err
	}
	c.InitDiffSize = size

	return nil
}

func (c *Container) calculateMountDiffSize() error {
	size, err := calculateFolderSize(loadedFilesystem.GetDiffFolderPath(*dockerLibPath, c.MountID))
	if err != nil {
		return err
	}
	c.MountDiffSize = size

	return nil
}

// locates and stores the init-id for the current container
func (c *Container) retrieveInitID() error {
	contents, err := readFileData(loadedFilesystem.GetContainerInitFilePath(*dockerLibPath, c.ContainerData.ID))
	if err != nil {
		return err
	}
	c.InitID = contents
	return nil
}

// locates and stores the mount-id for the current container
func (c *Container) retrieveMountID() error {
	contents, err := readFileData(loadedFilesystem.GetContainerMountFilePath(*dockerLibPath, c.ContainerData.ID))
	if err != nil {
		return err
	}
	c.MountID = contents
	return nil
}

// ContainerData represents the data that can be obtained from isuing
// docker info {container id}
type ContainerData struct {
	Name string `json:"Name"` // the name of the current running container
	ID   string `json:"Id"`   // the full id of the current running container
}

// performs a docker info command on the current container and retrieves the
// additional data needed
func (cd *ContainerData) getInfo() {}

// ContainerParent represents a single parent of a container currently existing
type ContainerParent struct {
	Hash              string     // the hash of the current parent
	IsLeaf            bool       // identifies whether the current parent is a leaf parent or not. A leaf parent will not have another parent
	ParentHash        string     // identifies what is the current's parent parent if there is any. Please see IsLeaf
	CacheDiffSize     int64      // identifies the size of the current layer diff
	CacheID           string     // identifies what is the current cache id for he current parent
	AssignedContainer *Container // checks to which container is the current layer allocated
}

func (cp *ContainerParent) init() {
	cp.getCacheID()
	cp.getCacheDiffSize()
	cp.getParent()
}

// retrieves the direct parent of the current layer
func (cp *ContainerParent) getParent() error {
	location := loadedFilesystem.GetLayerParentPath(*dockerLibPath, cp.Hash)

	// check if the parent file is there. If it is not, then this is a leaf and there is no more need
	// to continue scanning for parents
	if _, err := os.Stat(location); os.IsNotExist(err) {
		cp.IsLeaf = true
		return nil
	}

	contents, err := readFileData(location)
	if err != nil {
		return err
	}

	parentHash := getHashFromSha256(contents)
	cp.ParentHash = parentHash
	// create a new structure for another parent and allocate it
	parent := &ContainerParent{
		AssignedContainer: cp.AssignedContainer,
		Hash:              parentHash,
	}
	cp.AssignedContainer.Parents.PushBack(parent)
	parent.init()
	return nil
}

// retrieves the cache id for the current layer
func (cp *ContainerParent) getCacheID() error {
	contents, err := readFileData(loadedFilesystem.GetLayerCacheFilePath(*dockerLibPath, cp.Hash))
	if err != nil {
		return err
	}
	cp.CacheID = contents
	return nil
}

// retrieves the size of the diff folder for the cache
func (cp *ContainerParent) getCacheDiffSize() error {
	location := loadedFilesystem.GetDiffFolderPath(*dockerLibPath, cp.CacheID)
	size, err := calculateFolderSize(location)
	if err != nil {
		return err
	}
	cp.CacheDiffSize = size
	return nil
}

// calculateFolderSize calculates the size of a given folder
func calculateFolderSize(folderPath string) (int64, error) {
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

// readFileData reads a given file and returns the string representation of the contents of the file
func readFileData(filePath string) (string, error) {
	contents, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	return string(contents), nil
}

// getHashFromSha256 takes a string in any format. If the format starts with sha256:, then
// only the last part of the string is returned
func getHashFromSha256(str string) string {
	if strings.HasPrefix(string(str), "sha256:") {
		str = strings.Replace(str, "sha256:", "", -1)
	}

	return str
}
