package main

import (
	"fmt"
	"os"

	"github.com/ArpanSriv/gg/internal/cli"
)

func main() {
	if err := cli.Root().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "gg:", err)
		os.Exit(1)
	}
}
