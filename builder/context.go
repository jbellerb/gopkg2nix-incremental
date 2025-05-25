package main

import (
	"go/build"
	"log"
	"os"
	"runtime"
	"strconv"
)

var (
	// Go build context.
	Context = build.Default

	// Lazily initialized, temporary directory for generated files.
	buildDir string
)

func init() {
	// build.Default checks the environment for CGO_ENABLED, defaulting to true.
	// Since I don't support it, manually disable to avoid the source filter from
	// excluding non-cgo fallback files.
	Context.CgoEnabled = false
}

// BuildDir creates a shared temporary directory for build-related files. If
// BuildDir has already been called, it will return the same directory that was
// previously generated.
func BuildDir() string {
	if buildDir == "" {
		var err error
		buildDir, err = os.MkdirTemp(os.TempDir(), "builder")
		if err != nil {
			log.Fatalf("failed to initialize build directory: %v", err)
		}
	}

	return buildDir
}

// BuildParallelism returns the number of CPU cores Nix has asked us to use. If
// NIX_BUILD_CORES is not present, this is 1.
func BuildParallelism() int {
	cores := os.Getenv("NIX_BUILD_CORES")
	switch cores {
	case "":
		return 1
	case "0":
		return runtime.NumCPU()
	default:
		cores, err := strconv.ParseInt(cores, 10, 32)
		if err != nil {
			log.Fatalf("failed to parse NIX_BUILD_CORES: %v", err)
		}
		return int(cores)
	}
}
