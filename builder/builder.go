// builder is a wrapper around the Go compiler and linker for being called as a
// derivation builder within Nix. Builder translates the arguments passed to it
// from Nix with `__structuredAttrs` into the appropriate command line flags and
// calls the compiler or linker as appropriate.
package main

import (
	"fmt"
	"log"
	"nix/derivation"
	"os"
)

const (
	usage = `
Usage: builder [command]

Commands:
  compile
  link
  stdlib`
)

type Attrs struct {
	SDK             string
	GoCompatVersion string
}

// OutputPath looks up a derivation output and creates an empty directory there.
func OutputPath(output string) (string, error) {
	dir := derivation.Outputs[output]
	if dir == "" {
		return "", fmt.Errorf(
			"derivation was expected to produce an output \"%s\"",
			output,
		)
	}
	if err := os.Mkdir(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	return dir, nil
}

func main() {
	attrs := derivation.GetAttrs[Attrs]()

	if len(os.Args) < 2 {
		log.Fatalf("no subcommand provided\n%s", usage)
	}

	sdk, err := LoadSDK(attrs.SDK, attrs.GoCompatVersion)
	if err != nil {
		log.Fatalf(`failed to load sdk: %v

  Was "sdk" set in your derivation attributes?`, err)
	}

	command := os.Args[1]
	switch command {
	case "compile":
		compile(sdk)
	case "link":
		link(sdk)
	case "stdlib":
		stdlib(sdk)
	default:
		log.Fatalf("unknown command \"%s\"\n%s", command, usage)
	}
}
