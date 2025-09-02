{
  system,
  lib,
  go,
  useCaDerivations ? false,
}@pkgs:

let
  inherit (lib)
    mapAttrs
    mergeAttrsList
    optional
    optionalAttrs
    ;

  passthruDerivation =
    {
      passthru ? { },
      ...
    }@args:
    derivation (builtins.removeAttrs args [ "passthru" ]) // passthru;

in
rec {
  internal = {
    bootstrap = import ./bootstrap/default.nix {
      inherit system lib go;
      inherit buildGoBinary buildGoLibrary;
    };

    stdlib = import ./stdlib.nix {
      inherit system lib go;
      inherit builder buildGoLibrary;
      inherit (internal.bootstrap.stage2.stdlib) spec;
      inherit useCaDerivations;
    };

    derivation = buildGoLibrary {
      importPath = "nix/derivation";
      srcs = [
        ./internal/nix/derivation/attrs.go
        ./internal/nix/derivation/path.go
      ];
    };
  };

  /**
    The Go builder. See `buildGoBinary` for how it's used.
  */
  inherit (internal.bootstrap.stage2) builder;

  /**
    Compile a Go package into an archive usable as a member of `imports` in
    other builds.

    # Type

    ```
    buildGoLibrary
      :: { importPath :: String
         , srcs :: [String | Path]
         , imports :: [Derivation] ? []
         , packageName :: String ? baseNameOf importPath
         , importMap :: AttrSet ? {}
         , compileFlags :: [String] ? []
         , go :: Derivation ? pkgs.go
         , noStd :: Bool ? false
         }
      -> Derivation
    ```

    # Inputs

    An attribute set with the following arguments

    : `importPath` (String; _required_)
      : The import path of the package. This is what will appear for the
        "import" line when using the library.

    : `srcs` ([String | Path]; _required_)
      : Paths or store paths to the source files of the package. This must be
        individual files, not a directory of files.

    : `imports` ([Derivation]; optional, default: `[]`)
      : Other libraries depended on by the package. These must also be the
        output of `buildGoLibrary`.

    : `packageName` (String; optional, default: `baseNameOf importPath`)
      : The name of package. This is what is specified in the "package"
        statement and is used as the prefix for the libraries exports. By
        convention, this is the last element of the import path.

    : `importMap` (AttrSet; optional, default: `{}`)
      : Overrides for mapping import paths to Go packages. Usually this is only
        needed for vendored packages. The set should map from a string of the
        import path to a string of the real package path.

    : `compileFlags` ([String]; optional, default: `[]`)
      : Any extra flags to pass to the compiler.

    : `go` (Derivation; optional, default: `pkgs.go`)
      : The go compiler to use for building the binary. Note that the standard
        library will still be compiled against `pkgs.go` unless `noStd` is set.

    : `noStd` (Bool; optional, default: `false`)
      : Disable linking against the provided standard library. You must provide
        your own runtime and standard library as `imports`.
  */
  buildGoLibrary =
    {
      importPath,
      srcs,
      imports ? [ ],
      packageName ? builtins.baseNameOf importPath,
      compileFlags ? [ ],
      go ? pkgs.go,
      noStd ? false,
      ...
    }@args:
    let
      mergedDeps = mergeAttrsList (
        (builtins.map (dep: dep.deps // { "${dep.importPath}" = dep; }) imports)
        ++ optional (!noStd) { std = internal.stdlib.std; }
      );
    in
    passthruDerivation (
      {
        inherit system;
        name = builtins.replaceStrings [ "/" ] [ "_" ] importPath;

        __structuredAttrs = true;
        __contentAddressed = useCaDerivations;

        builder = "${builder}/bin/builder";
        args = [ "compile" ];
        outputs = [
          "lib"
          "export"
        ];

        sdk = "${go}/share/go";
        imports = builtins.listToAttrs (
          builtins.map (dep: {
            name = dep.importPath;
            value = dep.export;
          }) (imports ++ optional (!noStd) internal.stdlib.std)
        );
        inherit packageName compileFlags;

        passthru =
          (args.passthru or { })
          // {
            deps = mergedDeps;
          }
          // optionalAttrs (args ? "meta") { inherit (args) meta; };
      }
      // (builtins.removeAttrs args [
        "compileFlags"
        "go"
        "imports"
        "meta"
        "noStd"
        "packageName"
        "passthru"
      ])
    );

  /**
    Compile a Go package into a binary.

    # Type

    ```
    buildGoBinary
      :: { name :: String ? baseNameOf importPath
         , srcs :: [String | Path] ? obj.srcs
         , importPath :: String ? name
         , imports :: [Derivation] ? []
         , importMap :: AttrSet ? {}
         , compileFlags :: [String] ? []
         , obj :: Derivation | Null ? null
         , linkFlags :: [String] ? []
         , go :: Derivation ? pkgs.go
         , noStd :: Bool ? false
         }
      -> Derivation

    ```

    # Inputs

    An attribute set with the following arguments

    : `name` (String; optional, default: `baseNameOf importPath`)
      : Name of the output derivation. This must be set unless `importPath` is
        set.

    : `srcs` ([String | Path]; optional, default: `obj.srcs`)
      : Paths or store paths to the source files of the package. This must be
        individual files, not a directory of files. All files must be in the
        package "main".

    : `imports` ([Derivation]; optional, default: `[]`)
      : Other libraries depended on by the package. These must also be the
        output of `buildGoLibrary`.

    : `importPath` (String; optional, default: `name`)
      : The import path of the binary package. Usually this shouldn't be
        changed, but it is available if you need to import internal packages.

    : `importMap` (AttrSet; optional, default: `{}`)
      : Overrides for mapping import paths to Go packages. Usually this is only
        needed for vendored packages. The set should map from a string of the
        import path to a string of the real package path.

    : `compileFlags` ([String]; optional, default: `[]`)
      : Any extra flags to pass to the compiler.

    : `obj` (Derivation | Null; optional, default: `null`)
      : Completely override the compilation step and instead link the output of
        a call to `buildGoLibrary`.

    : `linkFlags` ([String]; optional, default: `[]`)
      : Any extra flags to pass to the linker.

    : `go` (Derivation; optional, default: `pkgs.go`)
      : The go compiler to use for building the library. Note that the standard
        library will still be compiled against `pkgs.go` unless `noStd` is set.

    : `noStd` (Bool; optional, default: `false`)
      : Disable linking against the provided standard library. You must provide
        your own runtime and standard library as `imports`.
  */
  buildGoBinary =
    {
      imports ? [ ],
      compileFlags ? [ ],
      linkFlags ? [ ],
      go ? pkgs.go,
      noStd ? false,
      ...
    }@args:
    let
      name =
        if args ? "importPath" then args.name or (builtins.baseNameOf args.importPath) else args.name;
      importPath = args.importPath or (builtins.parseDrvName args.name).name;

      obj =
        args.obj or (buildGoLibrary (
          {
            name = name + "_obj";

            inherit
              importPath
              imports
              compileFlags
              go
              noStd
              ;
            inherit (args) srcs;

            packageName = "main";
          }
          // optionalAttrs (args ? "importMap") { inherit (args) importMap; }
        ));
    in
    passthruDerivation (
      {
        inherit system name;

        __structuredAttrs = true;
        __contentAddressed = useCaDerivations;

        builder = "${builder}/bin/builder";
        args = args.linkArgs or [ "link" ];

        sdk = "${go}/share/go";

        inherit (obj) importPath;
        main = obj.export;
        inherit linkFlags;
        deps = mapAttrs (_: dep: dep.lib) (obj.deps // { "${obj.importPath}" = obj; });

        passthru = (args.passthru or { }) // {
          meta = args.meta or { } // {
            mainProgram = builtins.baseNameOf obj.importPath;
          };
        };
      }
      // (builtins.removeAttrs args [
        "compileFlags"
        "go"
        "importMap"
        "importPath"
        "imports"
        "linkArgs"
        "linkFlags"
        "meta"
        "name"
        "noStd"
        "obj"
        "packageName"
        "passthru"
        "srcs"
      ])
    );
}
