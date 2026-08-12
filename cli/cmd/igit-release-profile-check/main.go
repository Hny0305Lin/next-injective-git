package main

import (
	"fmt"
	"os"

	"github.com/Hny0305Lin/next-injective-git/cli/internal/config"
)

func main() {
	if err := config.ValidatePublishedProfilesV1Only(); err != nil {
		fmt.Fprintf(os.Stderr, "release profile check: fail: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release profile check: pass (published CLI profiles remain auto/v1)")
}
