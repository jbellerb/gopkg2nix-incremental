package derivation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	// The memoized path string.
	path string
)

// Path generates a colon-separated list of inputs, suitable for setting to
// $PATH.
func Path() string {
	var b strings.Builder

	for i, dep := range NativeBuildInputs {
		if i != 0 {
			fmt.Fprint(&b, ":")
		}
		fmt.Fprintf(&b, filepath.Join(dep, "bin"))
	}

	return b.String()
}

// SetPath sets the $PATH environment variable to the output of [Path].
func SetPath() error {
	return os.Setenv("PATH", Path())
}
