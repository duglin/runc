// +build linux

package main

import (
	"fmt"
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/opencontainers/runc/libcontainer"
	"github.com/opencontainers/specs"
)

func createContainer(context *cli.Context, spec *specs.LinuxSpec, rspec *specs.LinuxRuntimeSpec) (libcontainer.Container, error) {
	config, err := createLibcontainerConfig(context.GlobalString("id"), spec, rspec)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(config.Rootfs); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("Rootfs (%q) does not exist", config.Rootfs)
		}
		return nil, err
	}

	factory, err := loadFactory(context)
	if err != nil {
		return nil, err
	}
	container, err := factory.Create(context.GlobalString("id"), config)
	if err != nil {
		return nil, err
	}

	return container, nil
}

func deleteContainer(container libcontainer.Container) {
	status, err := container.Status()
	if err != nil {
		logrus.Error(err)
	}
	if status != libcontainer.Checkpointed {
		if err := container.Destroy(); err != nil {
			logrus.Error(err)
		}
	}
}

// runProcess will create a new process (PID ns) in the specified container
// by executing the process specified in the 'config'.
func runProcess(container libcontainer.Container, config *specs.Process, extraFDs []int, console string) (int, error) {
	process := newProcess(*config)

	// Add extra file descriptors if needed
	for _, i := range extraFDs {
		process.ExtraFiles = append(process.ExtraFiles, os.NewFile(uintptr(i), ""))
	}

	rootuid, err := container.Config().HostUID()
	if err != nil {
		return -1, err
	}

	tty, err := newTty(config.Terminal, process, rootuid, console)
	if err != nil {
		return -1, err
	}
	handler := newSignalHandler(tty)
	defer handler.Close()
	if err := container.Start(process); err != nil {
		return -1, err
	}
	return handler.forward(process)
}
