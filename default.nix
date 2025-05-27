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
      packagePath = "nix/derivation";
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
      :: { packagePath :: String
         , srcs :: [String | Path]
         , imports :: [Derivation] ? []
         , importMap :: AttrSet ? {}
         , compileFlags :: [String] ? []
         , go :: Derivation ? pkgs.go
         , noStd :: Bool ? false
         }
      -> Derivation
    ```

    # Inputs

    An attribute set with the following arguments

    : `packagePath` (String; _required_)
      : The name of package. This is what will appear for the "import" when
        using the library.

    : `srcs` ([String | Path]; _required_)
      : Paths or store paths to the source files of the package. This must be
        individual files, not a directory of files.

    : `imports` ([Derivation]; optional, default: `[]`)
      : Other libraries depended on by the package. These must also be the
        output of `buildGoLibrary`.

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
      packagePath,
      srcs,
      imports ? [ ],
      compileFlags ? [ ],
      go ? pkgs.go,
      noStd ? false,
      ...
    }@args:
    let
      mergedDeps = mergeAttrsList (
        (builtins.map (dep: dep.deps // { "${dep.packagePath}" = dep; }) imports)
        ++ optional (!noStd) { std = internal.stdlib.std; }
      );

    in
    derivation (
      {
        inherit system;
        name = builtins.replaceStrings [ "/" ] [ "_" ] "${packagePath}";

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
            name = dep.packagePath;
            value = dep.export;
          }) (imports ++ optional (!noStd) internal.stdlib.std)
        );
        inherit compileFlags;
      }
      // (builtins.removeAttrs args [
        "compileFlags"
        "go"
        "imports"
        "noStd"
      ])
    )
    // {
      deps = mergedDeps;
    };

  /**
    Compile a Go package into a binary.

    # Type

    ```
    buildGoBinary
      :: { name :: String
         , srcs :: [String | Path] ? obj.srcs
         , packagePath :: String ? "main"
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

    : `name` (String; _required_)
      : Name of the output derivation.

    : `srcs` ([String | Path]; optional, default: `obj.srcs`)
      : Paths or store paths to the source files of the package. This must be
        individual files, not a directory of files.

    : `imports` ([Derivation]; optional, default: `[]`)
      : Other libraries depended on by the package. These must also be the
        output of `buildGoLibrary`.

    : `packagePath` (String; optional, default: `main`)
      : The name of package the binary lives in. Usually this shouldn't be
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
      name,
      imports ? [ ],
      packagePath ? "main",
      compileFlags ? [ ],
      linkFlags ? [ ],
      go ? pkgs.go,
      noStd ? false,
      ...
    }@args:
    let
      main =
        args.obj or (buildGoLibrary (
          {
            inherit
              packagePath
              imports
              compileFlags
              go
              noStd
              ;
            inherit (args) srcs;
          }
          // optionalAttrs (args ? "importMap") { importMap = args.importMap or { }; }
        ));
    in
    derivation (
      {
        inherit system;

        __structuredAttrs = true;
        __contentAddressed = useCaDerivations;

        builder = "${builder}/bin/builder";
        args = args.linkArgs or [ "link" ];

        sdk = "${go}/share/go";

        inherit (main) packagePath;
        main = main.export;
        inherit name linkFlags;
        deps = mapAttrs (_: dep: dep.lib) (main.deps // { "${main.packagePath}" = main; });
      }
      // (builtins.removeAttrs args [
        "compileFlags"
        "go"
        "importMap"
        "imports"
        "linkArgs"
        "linkFlags"
        "name"
        "noStd"
        "obj"
        "packagePath"
      ])
    );
}
