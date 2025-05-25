package main

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"
	"nix/derivation"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type EmbedCfg struct {
	Patterns map[string][]string
	Files    map[string]string
}

type CompileAttrs struct {
	PackagePath string
	Srcs        []string
	Imports     map[string]string
	ImportMap   map[string]string
	EmbedCfg    *EmbedCfg

	CompileFlags []string
}

// sortSrcs sorts the Srcs list and splits it into Go files, header files, and
// assembly files.
func sortSrcs(srcs []string) (goSrcs, hSrcs, sSrcs []string, err error) {
	for _, src := range srcs {
		match, err := Context.MatchFile(filepath.Dir(src), filepath.Base(src))
		if err != nil {
			return nil, nil, nil, err
		}

		if !match {
			continue
		}

		switch filepath.Ext(src) {
		case ".go":
			goSrcs = append(goSrcs, src)
		case ".h":
			hSrcs = append(hSrcs, src)
		case ".s":
			sSrcs = append(sSrcs, src)
		default:
			log.Fatalf("source %s was neither a .go, .h, or .s file", src)
		}
	}

	slices.Sort(goSrcs)
	slices.Sort(hSrcs)
	slices.Sort(sSrcs)

	return
}

// compileImportCfg creates the importcfg neccesary for the Go compiler and
// returns the path to it, as well as a list of imports for writing the metadata
// later.
func compileImportCfg(
	srcs []string,
	deps map[string]string,
	importMap map[string]string,
) (string, []Import, error) {
	if err := ResolveMetaPackages(deps, importMap); err != nil {
		return "", nil, err
	}

	imports, rewrites, err := ScanImports(srcs, deps, importMap)
	if err != nil {
		return "", nil, err
	}

	cfgPath := filepath.Join(BuildDir(), "importcfg")
	cfgFile, err := os.Create(cfgPath)
	if err != nil {
		return "", nil, err
	}
	defer cfgFile.Close()

	for _, pkg := range rewrites {
		fmt.Fprintf(cfgFile, "importmap %s=%s\n", pkg.ImportPath, pkg.StorePath)
	}
	for _, pkg := range imports {
		fmt.Fprintf(
			cfgFile,
			"packagefile %s=%s/%s.x\n",
			pkg.ImportPath,
			pkg.StorePath,
			filepath.Base(pkg.ImportPath),
		)
	}

	return cfgPath, imports, nil
}

// packageTrimPath builds a "-trimpath" argument for the Go compiler to
// remove absolute file paths from the output binary. This is needed for
// reproducibility.
func packageTrimPath(srcs []string, importPath, out string) string {
	var trimPath strings.Builder
	srcPaths := make(map[string]struct{})

	for _, src := range srcs {
		dir := filepath.Dir(src)
		if _, ok := srcPaths[dir]; !ok {
			srcPaths[dir] = struct{}{}
			fmt.Fprintf(&trimPath, "%s=>%s;", dir, importPath)
		}
	}

	return trimPath.String() + out + "=>"
}

// findIncludes returns a sorted list of include directories for the header
// sources.
func findIncludes(sdkInclude string, hSrcs []string) []string {
	hDirs := map[string]struct{}{
		BuildDir(): {},
		sdkInclude: {},
	}
	for _, src := range hSrcs {
		dir := filepath.Dir(src)
		if strings.HasPrefix(dir, sdkInclude) {
			// Being an assembly header in the SDK is common enough to justify a special
			// case.
			continue
		}
		if _, ok := hDirs[dir]; !ok {
			hDirs[dir] = struct{}{}
		}
	}

	return slices.Sorted(maps.Keys(hDirs))
}

// symlinkArchHeaders symlinks architecture-specific (ending in _$GOOS or
// _$GOARCH) to the generic path (ending in _GOOS or _GOARCH). In other words,
// the value of $GOOS, for example "linux", is replaced with the literal string
// "GOOS".
func symlinkArchHeaders(hFiles []string) error {
	platformSuffix := "_" + HostPlatform + ".h"
	goosSuffix := "_" + Context.GOOS + ".h"
	goarchSuffix := "_" + Context.GOARCH + ".h"

	for _, path := range hFiles {
		base := filepath.Base(path)
		var newBase string

		if strings.HasSuffix(base, platformSuffix) {
			newBase = base[:len(base)-len(platformSuffix)] + "_GOOS_GOARCH.h"
		} else if strings.HasSuffix(base, goosSuffix) {
			newBase = base[:len(base)-len(goosSuffix)] + "_GOOS.h"
		} else if strings.HasSuffix(base, goarchSuffix) {
			newBase = base[:len(base)-len(goarchSuffix)] + "_GOARCH.h"
		}

		if newBase != "" {
			if err := os.Symlink(path, filepath.Join(BuildDir(), newBase)); err != nil {
				return err
			}
		}
	}

	return nil
}

// touchFile creates an empty file at path.
func touchFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	return file.Close()
}

// appendArchive adds object files to an archive.
func appendArchive(sdk *GoSDK, archive string, objs ...string) error {
	cmd := sdk.RunTool("pack", "r", archive)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	cmd.Args = append(cmd.Args, objs...)

	fmt.Fprintln(os.Stderr, cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pack archive: %w", err)
	}

	return nil
}

// hasForwardDecl contains hard-coded exceptions for packages in the standard
// library with forward declarations.
func hasForwardDecl(importPath string) bool {
	// List taken from src/cmd/go/internal/work/gc.go
	switch importPath {
	case "bytes", "internal/poll", "net", "os", "runtime/metrics":
		fallthrough
	case "runtime/pprof", "runtime/trace", "sync", "syscall", "time":
		return true
	default:
		return false
	}
}

// compileEmbedCfg creates the embedcfg neccesary for the Go compiler and
// returns the path to it.
func compileEmbedCfg(cfg *EmbedCfg) (string, error) {
	cfgPath := filepath.Join(BuildDir(), "embedcfg")
	cfgFile, err := os.Create(cfgPath)
	if err != nil {
		return "", err
	}
	defer cfgFile.Close()

	encoder := json.NewEncoder(cfgFile)
	return cfgPath, encoder.Encode(cfg)
}

// A Compilation represents a call to the Go compiler.
type Compilation struct {
	SDK        *GoSDK
	ImportPath string
	Srcs       []string
	Imports    map[string]string
	ImportMap  map[string]string
	EmbedCfg   *EmbedCfg

	goSrcs    []string
	hSrcs     []string
	sSrcs     []string
	includes  []string
	importCfg string
	imports   []Import
	trimPath  string
}

// Deps resolves a list of all packages directly imported by the compiled
// package, and all packages depended on by those imports. This must be called
// after the package has already been compiled.
func (c *Compilation) Deps() ([]string, []string, error) {
	if c.imports == nil {
		panic(
			"Compilation.Deps() called before Compilation.CompilePackage()",
		)
	}

	imports := make([]string, 0, len(c.imports))
	deps := make(map[string]struct{}, len(c.imports))
	for _, dep := range c.imports {
		imports = append(imports, dep.ImportPath)
		deps[dep.ImportPath] = struct{}{}

		pkg, err := LoadMetadata[Package](dep.StorePath, dep.ImportPath)
		if err != nil {
			return nil, nil, err
		}
		for _, subDep := range pkg.Deps {
			deps[subDep] = struct{}{}
		}
	}

	// imports should already be sorted by FindImports.
	return imports, slices.Sorted(maps.Keys(deps)), nil
}

// CompilePackage invokes the Go compiler to execute the Compilation.
func (c *Compilation) CompilePackage(
	obj string,
	exportData string,
	extraArgs []string,
) error {
	var err error
	c.goSrcs, c.hSrcs, c.sSrcs, err = sortSrcs(c.Srcs)
	if err != nil {
		return fmt.Errorf("failed to enumerate source files: %w", err)
	}

	c.importCfg, c.imports, err = compileImportCfg(
		c.goSrcs,
		c.Imports,
		c.ImportMap,
	)
	if err != nil {
		return fmt.Errorf("failed to generate compiler importcfg: %w", err)
	}

	c.trimPath = packageTrimPath(c.Srcs, c.ImportPath, filepath.Dir(obj))
	if len(c.sSrcs) > 0 {
		c.trimPath = c.trimPath + fmt.Sprintf(";%s=>", BuildDir())
	}

	cmd := c.SDK.RunTool("compile", extraArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = []string{"CGO_ENABLED=0"}

	cmd.Args = append(
		cmd.Args,
		"-o", exportData,
		"-linkobj", obj,
		"-trimpath", c.trimPath,
		"-p", c.ImportPath,
		"-lang", c.SDK.CompatVersion,
	)

	if len(c.sSrcs) > 0 {
		c.includes = findIncludes(c.SDK.Include(), c.hSrcs)
		asmHeader := filepath.Join(BuildDir(), "go_asm.h")
		if err := touchFile(asmHeader); err != nil {
			return err
		}
		if err := symlinkArchHeaders(c.hSrcs); err != nil {
			return err
		}
		symabis, err := c.AssembleSources(
			c.sSrcs,
			filepath.Join(BuildDir(), "symabis"),
			[]string{"-gensymabis"},
		)
		if err != nil {
			return err
		}
		cmd.Args = append(
			cmd.Args,
			"-symabis", symabis,
			"-asmhdr", asmHeader,
		)
	} else if !hasForwardDecl(c.ImportPath) {
		cmd.Args = append(cmd.Args, "-complete")
	}

	if c.EmbedCfg != nil {
		embedCfg, err := compileEmbedCfg(c.EmbedCfg)
		if err != nil {
			return fmt.Errorf("failed to generate compiler embedcfg: %w", err)
		}
		cmd.Args = append(cmd.Args, "-embedcfg", embedCfg)
	}

	cmd.Args = append(
		cmd.Args,
		"-c", fmt.Sprint(BuildParallelism()),
		"-nolocalimports",
		"-importcfg", c.importCfg,
		"-pack",
		"--",
	)
	cmd.Args = append(cmd.Args, c.goSrcs...)

	fmt.Fprintln(os.Stderr, cmd)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to compile binary: %w", err)
	}

	var sObjs []string
	for _, src := range c.sSrcs {
		base, _ := strings.CutSuffix(filepath.Base(src), ".s")
		obj, err := c.AssembleSources(
			[]string{src},
			filepath.Join(BuildDir(), fmt.Sprintf("%s.o", base)),
			[]string{},
		)
		if err != nil {
			return err
		}
		sObjs = append(sObjs, obj)
	}

	if sObjs != nil {
		return appendArchive(c.SDK, obj, sObjs...)
	}

	return nil
}

// AssembleSources assembles .s source file into an object for packing into the
// package archive.
func (c *Compilation) AssembleSources(
	srcs []string,
	out string,
	extraArgs []string,
) (string, error) {
	cmd := c.SDK.RunTool("asm", extraArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.Env = []string{"CGO_ENABLED=0"}

	cmd.Args = append(cmd.Args, "-p", c.ImportPath, "-trimpath", c.trimPath)
	for _, dir := range c.includes {
		cmd.Args = append(cmd.Args, "-I", dir)
	}
	cmd.Args = append(
		cmd.Args,
		"-D", fmt.Sprintf("GOOS_%s", Context.GOOS),
		"-D", fmt.Sprintf("GOARCH_%s", Context.GOARCH),
		"-o", out,
	)
	cmd.Args = append(cmd.Args, srcs...)

	fmt.Fprintln(os.Stderr, cmd)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to assemble sources: %w", err)
	}

	return out, nil
}

func compile(sdk *GoSDK) {
	attrs := derivation.GetAttrs[CompileAttrs]()

	libDir, err := OutputPath("lib")
	if err != nil {
		log.Fatal(err)
	}
	exportDir, err := OutputPath("export")
	if err != nil {
		log.Fatal(err)
	}

	name := filepath.Base(attrs.PackagePath)
	compilation := &Compilation{
		SDK:        sdk,
		ImportPath: attrs.PackagePath,
		Srcs:       attrs.Srcs,
		Imports:    attrs.Imports,
		ImportMap:  attrs.ImportMap,
		EmbedCfg:   attrs.EmbedCfg,
	}
	err = compilation.CompilePackage(
		filepath.Join(libDir, name+".a"),
		filepath.Join(exportDir, name+".x"),
		attrs.CompileFlags,
	)
	if err != nil {
		log.Fatal(err)
	}

	imports, deps, err := compilation.Deps()
	if err != nil {
		log.Fatalf("failed to collect dependencies: %v", err)
	}
	pkg := &Package{
		ImportPath: attrs.PackagePath,
		Imports:    imports,
		Deps:       deps,
	}
	if err := SaveMetadata(exportDir, pkg); err != nil {
		log.Fatalf("failed to generate package metadata: %v", err)
	}
}
