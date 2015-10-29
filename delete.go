package main

import (
	"os"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
)

var deleteCommand = cli.Command{
	Name:  "delete",
	Usage: "delete any resources held by the container often used with detached containers",
	Flags: []cli.Flag{},
	Action: func(context *cli.Context) {
		if os.Geteuid() != 0 {
			logrus.Fatal("runc should be run as root")
		}
		container, err := getContainer(context)
		if err != nil {
			logrus.Fatalf("Container delete failed: %v", err)
			os.Exit(-1)
		}
		deleteContainer(container)
	},
}
