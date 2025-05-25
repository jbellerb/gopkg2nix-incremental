// bootstrap is a tiny program that calls "go build" based on the "moduleName"
// and "out" environment variables.
//
// # Background
//
// builder compiles individual Go packages and links them, but as Nix
// derivations for proper caching. Since it is, by itself, written in Go,
// something is needed to process it before it can be used. The obvious answer
// is to run it on itself with "go run". Unfortunately, builder depends on
// other modules (and the standard library), which would need to be built in the
// specific format builder requires first.
//
// Go has a perfectly capable build system built in, we just don't get to take
// advantage of its caching from Nix. Even though builder doesn't use modules,
// module definitions can be written for builder and its dependencies. With a
// "go.work" pointing to all of them, Go's build system can build a functional
// copy of builder, but how would Go know to use it? These scripts all require
// __structuredAttrs which disables setting environment variables (like GOWORK)
// in a derivation. Instead, we will use another script. A small one, executed
// without structured attributes. That way Nix sets GOWORK and all the other
// required environment variables, which can then be passed to "go build".
//
// If we're not using structured attributes, why not call the Go compiler
// directly? "$out". Nix requires the output to be put in the output store
// location. there's no way to pass this value in as a command-line arg, since
// the path's value depends on the arguments list (among other things). As that
// can only be read through environment variables, some wrapper (shell script or
// this) is always neccessary.
package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

func main() {
	goBin := filepath.Join(os.Getenv("go"), "bin", "go")

	outDir := os.Getenv("out")
	modName := os.Getenv("moduleName")

	out := filepath.Join(outDir, "bin", "builder")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		log.Fatalf("failed to create bin directory: %v", err)
	}

	cmd := exec.Command(goBin, "build", "-o", out, modName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("failed to compile bootstrap binary: %v", err)
	}
}
