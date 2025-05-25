package main

import (
	"fmt"
	"log"
	"nix/derivation"
	"os"
	"path/filepath"
)

type LinkAttrs struct {
	PackagePath string
	Main        string
	Name        string
	Deps        map[string]string

	LinkFlags []string
}

// linkImportCfg creates the importcfg neccesary for the Go linker and returns
// the path to it, as well as the resolved main package.
func linkImportCfg(
	main *Package,
	mainPath string,
	deps map[string]string,
) (string, error) {
	if err := ResolveMetaPackages(deps, nil); err != nil {
		return "", err
	}

	imports := make([]Import, 0, len(main.Deps)+1)
	for _, importPath := range main.Deps {
		if storePath := deps[importPath]; storePath != "" {
			imports = append(imports, Import{storePath, importPath})
		} else {
			return "", &ImportError{importPath, main.ImportPath}
		}
	}
	imports = append(imports, Import{mainPath, "command-line-arguments"})
	SortImports(imports)

	cfgPath := filepath.Join(BuildDir(), "importcfg.link")
	cfgFile, err := os.Create(cfgPath)
	if err != nil {
		return "", err
	}
	defer cfgFile.Close()

	for _, pkg := range imports {
		fmt.Fprintf(
			cfgFile,
			"packagefile %s=%s/%s.a\n",
			pkg.ImportPath,
			pkg.StorePath,
			filepath.Base(pkg.ImportPath),
		)
	}

	return cfgPath, nil
}

// A Linkage represents a call to the Go linker.
type Linkage struct {
	SDK  *GoSDK
	Main Package
	Deps map[string]string

	importCfg string
}

// LinkPackage invokes the Go linker to execute the Linkage.
func (l *Linkage) LinkPackage(out string, extraArgs []string) error {
	storePath := l.Deps[l.Main.ImportPath]
	if storePath == "" {
		return &ImportError{l.Main.ImportPath, l.Main.ImportPath}
	}

	var err error
	l.importCfg, err = linkImportCfg(&l.Main, storePath, l.Deps)
	if err != nil {
		return fmt.Errorf("failed to generate linker importcfg: %w", err)
	}

	cmd := l.SDK.RunTool("link", extraArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = []string{
		"CGO_ENABLED=0",
		// Make sure GOROOT is unset.
		"GOROOT=",
	}

	cmd.Args = append(
		cmd.Args,
		"-o", out,
		"-importcfg", l.importCfg,
		"-buildmode", "exe",
		fmt.Sprintf("%s/%s.a", storePath, filepath.Base(l.Main.ImportPath)),
	)

	fmt.Fprintln(os.Stderr, cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to link binary: %w", err)
	}

	return nil
}

func link(sdk *GoSDK) {
	attrs := derivation.GetAttrs[LinkAttrs]()

	outDir, err := OutputPath("out")
	if err != nil {
		log.Fatal(err)
	}
	binDir := filepath.Join(outDir, "bin")
	if err := os.Mkdir(binDir, 0755); err != nil {
		log.Fatalf("failed to create bin directory: %v", err)
	}

	main, err := LoadMetadata[Package](attrs.Main, attrs.PackagePath)
	if err != nil {
		log.Fatalf("failed to load main module: %v", err)
	}

	linkage := &Linkage{
		SDK:  sdk,
		Main: main,
		Deps: attrs.Deps,
	}
	err = linkage.LinkPackage(filepath.Join(binDir, attrs.Name), attrs.LinkFlags)
	if err != nil {
		log.Fatal(err)
	}
}
