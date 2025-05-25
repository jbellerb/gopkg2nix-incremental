{
  lib,
  buildPlatform,
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
      inherit (buildPlatform) system;
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
      inherit lib buildPlatform go;
      inherit (stage1) builder;
      inherit buildGoLibrary;
    };

    derivation = buildGoLibrary {
      packagePath = "nix/derivation";
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
          packagePath = "main";

          srcs = [
            ../builder/builder.go
            ../builder/compile.go
            ../builder/context.go
            ../builder/link.go
            ../builder/package.go
            ../builder/sdk.go
            ../builder/stdlib.go
          ];
          imports = with stage2; [
            stage2.derivation
            stdlib."encoding/json"
            stdlib.fmt
            stdlib."go/build"
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
          ];

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
