{
  system,
  lib,
  go,
  buildGoBinary,
  buildGoLibrary,
  useCaDerivations ? false,
}:

let
  inherit (lib) fileset;

in
rec {
  stage1 = {
    builder = derivation {
      inherit system;
      name = "builder-stage1";

      __contentAddressed = useCaDerivations;

      builder = "${go}/bin/go";
      args = [
        "run"
        "${./bootstrap.go}"
      ];

      GOCACHE = "/tmp/go-cache";
      GOPATH = "/tmp/go";
      GOSUMDB = "off";
      GOWORK =
        let
          workspaceDir = fileset.toSource {
            root = ../.;
            fileset = fileset.unions [
              ./cmd
              ./go.work
              ./pkg/nix
              ../builder
              ../internal/nix/derivation
            ];
          };
        in
        "${workspaceDir}/bootstrap/go.work";

      inherit go;
      moduleName = "cmd/builder";
    };
  };

  stage2 = {
    stdlib = import ../stdlib.nix {
      inherit system lib go;
      inherit (stage1) builder;
      inherit buildGoLibrary;
    };

    derivation = buildGoLibrary {
      importPath = "nix/derivation";
      srcs = [
        ../internal/nix/derivation/attrs.go
        ../internal/nix/derivation/path.go
      ];
      imports = with stage2; [
        stdlib."encoding/json"
        stdlib.fmt
        stdlib.log
        stdlib.os
        stdlib."path/filepath"
        stdlib.strings
      ];

      noStd = true;
      builder = "${stage1.builder}/bin/builder";
    };

    builder =
      let
        obj = buildGoLibrary {
          importPath = "builder";
          srcs = [
            ../builder/builder.go
            ../builder/compile.go
            ../builder/context.go
            ../builder/link.go
            ../builder/package.go
            ../builder/sdk.go
            ../builder/stdlib.go
            ../builder/test.go
          ];
          imports = with stage2; [
            stage2.derivation
            stdlib.cmp
            stdlib."encoding/json"
            stdlib.fmt
            stdlib."go/ast"
            stdlib."go/build"
            stdlib."go/doc"
            stdlib."go/parser"
            stdlib."go/token"
            stdlib.io
            stdlib.log
            stdlib.maps
            stdlib.os
            stdlib."os/exec"
            stdlib."path/filepath"
            stdlib.runtime
            stdlib.slices
            stdlib.strconv
            stdlib.strings
            stdlib.sync
            stdlib."text/template"
          ];

          packageName = "main";
          noStd = true;
          builder = "${stage1.builder}/bin/builder";
        };
      in
      buildGoBinary {
        name = "builder";
        inherit obj;

        linkFlags = [
          "-X"
          "'nix/derivation.Name=builder'"
        ];

        builder = "${stage1.builder}/bin/builder";
      };
  };
}
