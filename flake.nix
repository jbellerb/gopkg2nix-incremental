{
  description = "Demonstration of incremental Go compilation in Nix";

  outputs =
    { self }:
    {
      overlays.default = import ./overlay.nix;
    };
}
