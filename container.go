// +build linux

package main

import (
	"fmt"
	"os"
	"syscall"

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

	if err := container.Create(newProcess(spec.Process)); err != nil {
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
func runProcess(container libcontainer.Container, config *specs.Process, extraFDs []int, console string, pidFile string, detach bool) (int, error) {
	process := newProcess(*config)

	// Add extra file descriptors if needed
	for _, i := range extraFDs {
		process.ExtraFiles = append(process.ExtraFiles, os.NewFile(uintptr(i), ""))
	}

	rootuid, err := container.Config().HostUID()
	if err != nil {
		return -1, err
	}

	var tty *tty
	if config.Terminal {
		if tty, err = createTty(process, rootuid, console); err != nil {
			return -1, err
		}
	} else if detach {
		if err := dupStdio(process, rootuid); err != nil {
			return -1, err
		}
	} else {
		if tty, err = createStdioPipes(process, rootuid); err != nil {
			return -1, err
		}
	}

	if err := container.Start(process); err != nil {
		return -1, err
	}

	if pidFile != "" {
		pid, err := process.Pid()
		if err != nil {
			return -1, err
		}
		f, err := os.Create(pidFile)
		if err != nil {
			logrus.WithField("pid", pid).Error("create pid file")
		} else {
			_, err = fmt.Fprintf(f, "%d", pid)
			f.Close()
			if err != nil {
				logrus.WithField("error", err).Error("write pid file")
			}
		}
	}
	if detach {
		return 0, nil
	}
	handler := newSignalHandler(tty)
	defer handler.Close()

	return handler.forward(process)
}

func dupStdio(process *libcontainer.Process, rootuid int) error {
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	for _, fd := range []uintptr{
		os.Stdin.Fd(),
		os.Stdout.Fd(),
		os.Stderr.Fd(),
	} {
		if err := syscall.Fchown(int(fd), rootuid, rootuid); err != nil {
			return err
		}
	}
	return nil
}
