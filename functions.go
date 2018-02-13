package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"plugin"
	"strconv"
	"strings"

	stackerrors "github.com/go-errors/errors"
)

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
	if _, err := os.Stat(folderPath); err != nil {
		return 0, stackerrors.Wrap(err, 1)
	}
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
		return 0, stackerrors.Wrap(err, 1)
	}

	return size, nil
}

// GetAllContainers returns all the containers currently running in the target server
func GetAllContainers(filesystemPlugin FilesystemPather) ([]*Container, error) {
	files, err := ioutil.ReadDir(*fsPath + "/containers/")
	if err != nil {
		return nil, stackerrors.Wrap(err, 1)
	}

	containers := []*Container{}
	for _, file := range files {
		container := &Container{
			Hash:       file.Name(),
			Filesystem: filesystemPlugin,
		}
		err := container.Init()
		if err != nil {
			log.Println("File " + file.Name() + " does not have an associated container, skipping...")
			continue
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
