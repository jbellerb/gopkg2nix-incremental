final: prev:

let
  inherit (prev.lib) makeExtensible;

  goLib = import ./default.nix {
    inherit (prev.stdenv.buildPlatform) system;
    inherit (prev) lib go;
  };

in
{
  gopkg2nix-builder = goLib.builder;

  goPackages = makeExtensible (
    final: goLib.internal.stdlib // { "nix/derivation" = goLib.internal.derivation; }
  );

  inherit (goLib) buildGoLibrary buildGoBinary;
}
