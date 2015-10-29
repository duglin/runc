// +build linux

package main

import (
	"os"
	"strconv"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/opencontainers/specs"
)

var runCommand = cli.Command{
	Name:  "run",
	Usage: "create and run a single command in a new container",
	Flags: []cli.Flag{
		cli.StringFlag{
			Name:  "config-file, c",
			Value: "config.json",
			Usage: "path to spec config file",
		},
		cli.StringFlag{
			Name:  "runtime-file, r",
			Value: "runtime.json",
			Usage: "path to runtime config file",
		},
	},
	Action: func(context *cli.Context) {
		spec, rspec, err := loadSpec(context.String("config-file"), context.String("runtime-file"))
		if err != nil {
			fatal(err)
		}

		notifySocket := os.Getenv("NOTIFY_SOCKET")
		if notifySocket != "" {
			setupSdNotify(spec, rspec, notifySocket)
		}

		listenFds := os.Getenv("LISTEN_FDS")
		listenPid := os.Getenv("LISTEN_PID")

		if listenFds != "" && listenPid == strconv.Itoa(os.Getpid()) {
			setupSocketActivation(spec, listenFds)
		}

		if os.Geteuid() != 0 {
			logrus.Fatal("runc should be run as root")
		}
		status, err := runContainer(context, spec, rspec, context.Args())
		if err != nil {
			logrus.Fatalf("Container start failed: %v", err)
		}

		// exit with the container's exit status so any external supervisor is
		// notified of the exit with the correct exit status.
		os.Exit(status)
	},
}

func runContainer(context *cli.Context, spec *specs.LinuxSpec, rspec *specs.LinuxRuntimeSpec, args []string) (int, error) {
	container, err := createContainer(context, spec, rspec)
	if err != nil {
		return -1, err
	}
	defer deleteContainer(container) // delete when process dies

	// Support on-demand socket activation by passing file descriptors into the container init process.
	extraFDs := []int{}

	if fds := os.Getenv("LISTEN_FDS"); fds != "" {
		listenFdsInt, err := strconv.Atoi(fds)
		if err != nil {
			return -1, err
		}

		for i := 0; i < listenFdsInt; i++ {
			extraFDs = append(extraFDs, SD_LISTEN_FDS_START+i)
		}
	}

	proc := specs.Process{
		Terminal: spec.Process.Terminal,
		User:     spec.Process.User,
		Args:     args,
		Env:      spec.Process.Env,
		Cwd:      spec.Process.Cwd,
	}

	return runProcess(container, &proc, extraFDs, context.String("console"))
}
