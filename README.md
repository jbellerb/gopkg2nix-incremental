# gopkg2nix-incremental

This library implements a prototype of an alternative to nixpkgs'
`buildGoModule` capable of incremental compilation. By directly calling
the Go compiler and linker (instead of the Go build system—i.e. the `go`
command), individual packages can be built as derivations to better leverage
Nix's robust caching system. With the addition of the experimental "[floating
content-addressed derivations](https://github.com/NixOS/nix/issues/4087)"
feature, builds can further be cached to the level of package export data,
matching the efficiency of
[Google's Blaze](https://jayconrod.com/posts/112/export-data--the-secret-of-go-s-fast-builds).

Builds are initially slower due to the significant overhead of building and
tearing down sandboxes while realizing derivations, but subsequent builds
are substantially faster than any other Nix-based approach I'm aware of—even
without `ca-derivations` enabled. The downside to this approach is that Nix
must be aware of the full package build graph at evaluation time (at least until
[dynamic derivations](https://github.com/NixOS/nix/issues/6316) matures). For
now, I have resorted to manually specifying dependencies, but this could be
automated with a code generator similar to Bazel's
[Gazelle](https://github.com/bazel-contrib/bazel-gazelle) or Meta's
[Reindeer](https://github.com/facebookincubator/reindeer).

> [!WARNING]
> This is not production software. Much of this library was spun-off from an
> unrelated personal project and is being shared because I was told others would
> find the idea useful. If you are interested in building a more serious tool
> based on this, please reach out!

<br />

#### License

<sup>
Copyright (C) jae beller, 2025.
</sup>
<br />
<sup>
Released under the <a href="https://www.gnu.org/licenses/lgpl-3.0.txt">GNU Lesser General Public License, Version 3.0</a> or later. See <a href="LICENSE">LICENSE</a> for more information.
</sup>
