package cmd

import "miniDocker/container"

func Run(args []string) error { return container.RunContainer(args) }
