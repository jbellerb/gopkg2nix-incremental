final: prev:

let
  inherit (prev.lib) makeExtensible;

  goLib = import ./default.nix {
    inherit (prev.stdenv.buildPlatform) system;
    inherit (prev) lib go;
    useCaDerivations =
      prev.config.contentAddressedByDefault || (prev.config.contentAddressedGoPackages or false);
  };

in
{
  # # There is no way to set new configuration options from an overlay, because
  # # nixpkgs is hard-coded to load them from pkgs/top-level/config.nix.
  # # Fortunately, the module for determining the config takes a `freeformType` so
  # # this config attribute can still be specified, it just won't be typed.
  # config.contentAddressedGoPackages = mkOption {
  #   type = types.bool;
  #   default = false;
  #   description = ''
  #     Whether to build Go packages with the experimental `__contentAddressed`
  #     feature enabled.

  #     This can lead to significant savings during Go package builds, since
  #     dependent packages only need to be rebuilt if the modified package's
  #     public API changes. `experimental-features = ca-derivations` must be in
  #     your `nix.conf` for floating content addressed derivations to work.
  #   '';
  # };

  gopkg2nix-builder = goLib.builder;

  goPackages = makeExtensible (
    final: goLib.internal.stdlib // { "nix/derivation" = goLib.internal.derivation; }
  );

  inherit (goLib) buildGoLibrary buildGoBinary;
}
