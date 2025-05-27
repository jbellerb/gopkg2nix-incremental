// Package derivation implements helpers for Nix derivation builders to read
// their derivation's inputs using "__structuredAttrs".
//
// Programs should only import this if they are intended to be used as a
// builder. The calling derivation must be built with "__structuredAttrs
// = true".
package derivation

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

var (
	// The name of the builder executable.
	Name = "builder"

	// Raw JSON of the derivation attributes. Provided from Nix via
	// __structuredAttrs.
	AttrJson []byte

	// The expected outputs of the derivation and their store paths.
	Outputs map[string]string

	// The host-specific input packages.
	NativeBuildInputs []string
)

// Instead of requiring consumers to include these attributes in their own
// parsed attrs struct, parse them during "init" and expose them as package
// variables.
type wellKnownAttrs struct {
	Outputs map[string]string `json:"outputs"`

	NativeBuildInputs []string `json:"nativeBuildInputs"`
}

func init() {
	log.SetFlags(0)
	log.SetPrefix(fmt.Sprintf("%s: ", Name))

	file := os.Getenv("NIX_ATTRS_JSON_FILE")
	if file == "" {
		log.Fatal(`failed to locate $NIX_ATTRS_JSON_FILE

  Is this builder being called as a builder for a derivation?`)
	}

	var err error
	if AttrJson, err = os.ReadFile(file); err != nil {
		log.Fatalf("failed to read $NIX_ATTRS_JSON_FILE: %v", err)
	}

	// Parse the well-known attributes and copy them to vars.
	attrs := GetAttrs[wellKnownAttrs]()
	Outputs = attrs.Outputs
	NativeBuildInputs = attrs.NativeBuildInputs
}

// GetAttrs loads and parses structured attributes from the Nix derivation
// inputs. The provided type should support Unmarshalling from JSON.
func GetAttrs[T any]() T {
	var attrs T
	if err := json.Unmarshal(AttrJson, &attrs); err != nil {
		log.Fatalf("failed to parse attributes: %v", err)
	}
	return attrs
}
