package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	// The current host platform. This is "$GOOS_$GOARCH".
	HostPlatform = fmt.Sprintf("%s_%s", Context.GOOS, Context.GOARCH)
)

// GoSDK holds information about a specific instance of the Go SDK.
type GoSDK struct {
	// Path to the Go SDK. This is usually $GOROOT.
	Path string

	// Version of the Go SDK.
	Version string

	// User-requested version to maintain compatibility with.
	CompatVersion string
}

// ShortVersion returns the "major.minor" of the SDK, without the patch number.
func (sdk *GoSDK) ShortVersion() string {
	dot := strings.LastIndex(sdk.Version, ".")
	if dot == -1 {
		// Is there a better way to indicate this failure?
		return sdk.Version
	}
	return sdk.Version[0:dot]
}

// Include returns the "pkg/include" directory of the SDK.
func (sdk *GoSDK) Include() string {
	return filepath.Join(sdk.Path, "pkg", "include")
}

// LoadSDK loads information about a copy of the Go SDK and creates a [GoSDK]
// handle for using it.
func LoadSDK(path, compat string) (*GoSDK, error) {
	sdk := GoSDK{Path: path, CompatVersion: compat}

	version, err := sdk.compilerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to parse go compiler version: %w", err)
	}
	sdk.Version = version
	if sdk.CompatVersion == "" {
		sdk.CompatVersion = "go" + sdk.ShortVersion()
	}

	return &sdk, nil
}

// RunTool creates a new exec.Cmd for calling a given tool in the Go SDK.
func (sdk *GoSDK) RunTool(tool string, args ...string) *exec.Cmd {
	toolBin := filepath.Join(sdk.Path, "pkg", "tool", HostPlatform, tool)

	return exec.Command(toolBin, args...)
}

// RunGo creates a new exec.Cmd for calling the main "go" binary.
func (sdk *GoSDK) RunGo(args ...string) *exec.Cmd {
	goBin := filepath.Join(sdk.Path, "bin", "go")

	return exec.Command(goBin, args...)
}

// compilerVersion parses the Go compiler version from the output of "go
// version".
func (sdk *GoSDK) compilerVersion() (string, error) {
	infoBytes, err := sdk.RunGo("version").CombinedOutput()
	if err != nil {
		return "", err
	}
	info := string(infoBytes)

	// Something like "go version go1.23.5 linux/amd64"
	fields := strings.Fields(info)
	if len(fields) < 3 && fields[0] != "go" && fields[1] != "version" {
		return "", fmt.Errorf("malformed output \"%s\"", info)
	}
	fullVersion := fields[2]
	version, _ := strings.CutPrefix(fullVersion, "go")
	return version, nil
}
