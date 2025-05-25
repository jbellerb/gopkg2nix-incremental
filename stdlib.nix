{
  system,
  lib,
  go,
  builder,
  buildGoLibrary,
  useCaDerivations ? false,
  ...
}@args:

let
  inherit (lib)
    importJSON
    mapAttrs
    mergeAttrsList
    nameValuePair
    optionalAttrs
    ;

  specFile = derivation {
    inherit system;
    name = "std-spec";

    __structuredAttrs = true;
    __contentAddressed = useCaDerivations;

    builder = "${builder}/bin/builder";
    args = [
      "stdlib"
      "list"
    ];

    sdk = "${go}/share/go";
  };

  # IFD, but since it's only once at the beginning it shouldn't slow things
  # down much.
  spec = args.spec or (importJSON "${specFile}/spec.json");

  pkgs = builtins.listToAttrs (
    builtins.map (pkg: {
      name = pkg.ImportPath;
      value = buildGoLibrary (
        {
          packagePath = pkg.ImportPath;
          srcs = builtins.map (file: "${go}/share/go/src/${pkg.ImportPath}/${file}") (
            pkg.GoFiles or [ ] ++ pkg.HFiles or [ ] ++ pkg.SFiles or [ ]
          );
          imports = builtins.map (dep: pkgs."${dep}") (pkg.Imports or [ ]);

          compileFlags = [ "-std" ];

          noStd = true;
          builder = "${builder}/bin/builder";
        }
        // optionalAttrs (pkg ? "ImportMap") { importMap = pkg.ImportMap or { }; }
        // optionalAttrs (pkg ? "EmbedPatterns" && pkg ? "EmbedFiles") {
          embedCfg = {
            Patterns = builtins.listToAttrs (
              builtins.map (pattern: nameValuePair pattern [ pattern ]) (pkg.EmbedPatterns or [ ])
            );
            Files = builtins.listToAttrs (
              builtins.map (file: {
                name = file;
                value = "${go}/share/go/src/${pkg.ImportPath}/${file}";
              }) (pkg.EmbedFiles or [ ])
            );
          };
        }
      );
    }) spec
  );

in
pkgs
// {
  inherit spec;

  std =
    derivation {
      inherit system;
      name = "std-obj";

      __structuredAttrs = true;
      __contentAddressed = useCaDerivations;

      builder = "${builder}/bin/builder";
      args = [
        "stdlib"
        "package"
      ];
      outputs = [
        "lib"
        "export"
      ];

      sdk = "${go}/share/go";
      packages = mapAttrs (_: pkg: { inherit (pkg) lib export; }) pkgs;
      importMap = mergeAttrsList (builtins.map (dep: dep.importMap or { }) (builtins.attrValues pkgs));
    }
    // {
      packagePath = "std";
    };
}
