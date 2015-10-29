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
			Name:  "bundle, b",
			Value: "",
			Usage: "path to the root of the bundle directory",
		},
		cli.StringFlag{
			Name:  "console",
			Value: "",
			Usage: "specify the pty slave path for use with the container",
		},
		cli.BoolFlag{
			Name:  "detach,d",
			Usage: "detach from the container's process",
		},
		cli.StringFlag{
			Name:  "pid-file",
			Value: "",
			Usage: "specify the file to write the process id to",
		},
	},
	Action: func(context *cli.Context) {
		bundle := context.String("bundle")
		if bundle != "" {
			if err := os.Chdir(bundle); err != nil {
				fatal(err)
			}
		}

		spec, rspec, err := loadSpec(specConfig, runtimeConfig)
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

	detach := context.Bool("detach")
	if !detach {
		defer deleteContainer(container) // delete when process dies
	}

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

	return runProcess(container, &proc, extraFDs, context.String("console"), context.String("pid-file"), detach)
}
