package main

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var (
	// MetaPackages are special import paths which represent a commonly used set of
	// packages.
	MetaPackages = []string{"std"}
)

// ImportError records when an input could not be found in the current
// imports.
type ImportError struct {
	Import string
	Parent string
}

func (e ImportError) Error() string {
	return fmt.Sprintf(
		"package %s not found in the provided imports, needed by %s",
		e.Import,
		e.Parent,
	)
}

// FilterInternalPackages returns true if a package named importPath is an
// internal package that should be filtered from the output.
func FilterInternalPackages(importPath string) bool {
	switch importPath {
	case "runtime/cgo", "unsafe":
		return true
	default:
		return false
	}
}

// Interface PackageLike represents the metadata of either a package or meta
// package. This generalizes loading and saving .json descriptions to the store.
type PackageLike[P any] interface {
	StorePath(string) string
	FromImport(string) P
}

// SaveMetadata writes the metadata for a package-like object to a store path.
func SaveMetadata[T PackageLike[T]](dir string, data PackageLike[T]) error {
	file, err := os.Create(data.StorePath(dir))
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	return encoder.Encode(data)
}

// LoadMetadata loads the metadata for a single package-like object from a store
// path.
func LoadMetadata[T PackageLike[T]](dir string, importPath string) (T, error) {
	var pkg T
	pkg = pkg.FromImport(importPath)

	file, err := os.Open(filepath.Join(dir, filepath.Base(importPath)+".json"))
	if err != nil {
		return pkg, fmt.Errorf("failed to read package metadata: %w", err)
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	return pkg, decoder.Decode(&pkg)
}

type Package struct {
	ImportPath string `json:"-"`
	Imports    []string
	Deps       []string
}

func (p Package) StorePath(dir string) string {
	return filepath.Join(dir, filepath.Base(p.ImportPath)+".json")
}
func (p Package) FromImport(path string) Package {
	return Package{ImportPath: path}
}

type Import struct {
	StorePath  string
	ImportPath string
}

// SortImports sorts a slice of imports lexicographically by import path.
func SortImports(imports []Import) {
	slices.SortFunc(imports, func(a, b Import) int {
		return strings.Compare(a.ImportPath, b.ImportPath)
	})
}

type MetaPackage struct {
	ImportPath  string `json:"-"`
	SubPackages []Import
	ImportMap   map[string]string `json:",omitempty"`
}

func (p MetaPackage) StorePath(dir string) string {
	return filepath.Join(dir, filepath.Base(p.ImportPath)+".json")
}
func (p MetaPackage) FromImport(path string) MetaPackage {
	return MetaPackage{ImportPath: path}
}

// ResolveMetaPackages replaces known meta packages with their subpackages.
func ResolveMetaPackages(
	pkgs map[string]string,
	importMap map[string]string,
) error {
	for _, importPath := range MetaPackages {
		if storePath := pkgs[importPath]; storePath != "" {
			// Only allocate a new map if a meta package is actually found.
			if pkgs == nil {
				pkgs = maps.Clone(pkgs)
			}
			delete(pkgs, importPath)

			name := filepath.Base(importPath)
			file, err := os.Open(filepath.Join(storePath, name+".json"))
			if err != nil {
				return fmt.Errorf("failed to read meta package: %w", err)
			}
			defer file.Close()

			var pkg MetaPackage
			decoder := json.NewDecoder(file)
			if err := decoder.Decode(&pkg); err != nil {
				return fmt.Errorf("failed to read %s: %w", importPath, err)
			}

			for _, subPkg := range pkg.SubPackages {
				// If a package already exists, it was declared manually by the user. It
				// should override the declaration in the meta package.
				if _, ok := pkgs[subPkg.ImportPath]; !ok {
					pkgs[subPkg.ImportPath] = subPkg.StorePath
				}
			}
			if importMap != nil {
				maps.Copy(importMap, pkg.ImportMap)
			}
		}
	}

	return nil
}

// listFileImports parses a .go file and returns a list of all package paths
// imported by the file.
func listFileImports(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, file, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}

	var imports []string
	for _, pkg := range parsed.Imports {
		unquoted, err := strconv.Unquote(pkg.Path.Value)
		if err != nil {
			err = fmt.Errorf("parse input at %s: %v", fset.Position(pkg.Pos()), err)
			return nil, err
		}

		imports = append(imports, unquoted)
	}

	return imports, nil
}

// ScanImports searches through a list of files and resolves each import to
// its export data. If any imports were rewritten by the import map, an import
// for the original import path pointing to the rewritten path is added to
// the second list of imports. Both returned lists are already sorted.
func ScanImports(
	srcs []string,
	pkgs map[string]string,
	importMap map[string]string,
) ([]Import, []Import, error) {
	imports := make([]Import, 0)
	rewrites := make([]Import, 0)

	found := make(map[string]struct{})
	for _, path := range srcs {
		fileImports, err := listFileImports(path)
		if err != nil {
			return nil, nil, err
		}

		for _, importPath := range fileImports {
			if _, ok := found[importPath]; ok {
				continue
			}

			found[importPath] = struct{}{}
			if FilterInternalPackages(importPath) {
				continue
			}

			if truePath := importMap[importPath]; truePath != "" {
				rewrites = append(rewrites, Import{truePath, importPath})
				importPath = truePath
			}
			if storePath, ok := pkgs[importPath]; ok {
				imports = append(imports, Import{storePath, importPath})
			} else {
				return nil, nil, &ImportError{Import: importPath, Parent: path}
			}
		}
	}

	SortImports(imports)
	SortImports(rewrites)

	return imports, rewrites, nil
}
