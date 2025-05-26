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

## Usage

The easiest way to include gopkg2nix-incremental in your project is as a nixpkgs
overlay. The overlay can be imported traditionally from overlay.nix or with
flakes as `gopkg2nix-incremental.overlays.default`. If not using the overlay,
gopkg2nix-incremental can be instantiated by calling it as a function with
an attribute set containing these attributes: `system`, the system string;
`lib`, an instance of nixpkgs/lib; and `go`, a derivation for the Go compiler
toolchain.

<details>
<summary>Example: Importing gopkg2nix-incremental in a flake</summary>

<br />

```nix
{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    gopkg2nix-incremental.url = "github:jbellerb/gopkg2nix-incremental";
  };

  outputs =
    { self, nixpkgs, ... }@inputs:
    let
      systems = [
        "x86_64-linux"
        "aarch64-darwin"
        "x86_64-darwin"
      ];
    in
    builtins.foldl' nixpkgs.lib.recursiveUpdate { } (
      builtins.map (
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ inputs.gopkg2nix-incremental.overlays.default ];
          };

          # or if not using an overlay
          goLib = import inputs.gopkg2nix-incremental {
            inherit system;
            inherit (pkgs) lib go;
          };

        in
        {
          ...
        }
      ) systems
    );
}
```

</details>

Once imported, usage is largely similar to Bazel's `rules_go`. Building any
project is done with a combination of two functions: `buildGoLibrary` for
compiling library packages that can be used as dependencies, and `buildGoBinary`
for compiling and linking executable packages.

Both functions take a `srcs` argument which refers to a list of source files
(either Go, Assembly, or Headers. `go:embed` is not directly supported). These
source files can refer to packages specified by the `inputs` argument, a list of
dependencies. The Go standard library is automatically included as a dependency
unless `noStd = true`. All dependencies in this list must be the result of
a call to `buildGoLibrary`. Finally, `buildGoLibrary` takes the argument
`packagePath` for the path used in Go when importing that package. Equivalently,
`buildGoLibrary` takes the argument `name` for the name of the derivation and
the output binary.

<details>
<summary>Example: Building an executable with an external dependency</summary>

<br />

In this example we will build a simple command line utility that converts
between currencies using the foreign exchange data published by the
[European Central Bank](https://www.ecb.europa.eu/stats/policy_and_exchange_rates/euro_reference_exchange_rates/html/index.en.html).
Create a `flake.nix` for compiling the binary:

```nix
{
  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    gopkg2nix-incremental.url = "github:jbellerb/gopkg2nix-incremental";
  };

  outputs =
    { self, nixpkgs, ... }@inputs:
    let
      systems = [
        "x86_64-linux"
        "aarch64-darwin"
        "x86_64-darwin"
      ];
    in
    builtins.foldl' nixpkgs.lib.recursiveUpdate { } (
      builtins.map (
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
            overlays = [ inputs.gopkg2nix-incremental.overlays.default ];
          };

        in
        {
          packages."${system}".default = pkgs.buildGoBinary {
            name = "currency";
            srcs = [ ./currency.go ];
          };
        }
      ) systems
    );
}
```

And now the source of the program, `currency.go`:

```go
package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"net/http"
)

const source = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"

type ReferenceRates struct {
	Date       string `xml:"time,attr"`
	Currencies []struct {
		Name string  `xml:"currency,attr"`
		Rate float64 `xml:"rate,attr"`
	} `xml:"Cube"`
}

var rates = map[string]float64{"EUR": 1.0}

func currencyRate(name string) float64 {
	value, ok := rates[name]
	if !ok {
		log.Fatalf("unsupported currency: %s", name)
	}
	return value
}

func main() {
	log.SetFlags(0)
	from := flag.String("f", "USD", "currency to convert from")
	to := flag.String("t", "EUR", "currency to convert to")
	n := flag.Float64("n", 1.0, "quantity of currency to convert")
	flag.Parse()

	res, err := http.Get(source)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	decoder := xml.NewDecoder(res.Body)

	var envelope struct {
		Data ReferenceRates `xml:"Cube>Cube"`
	}
	if err := decoder.Decode(&envelope); err != nil {
		log.Fatalf("failed to parse currency data: %v", err)
	}
	for _, currency := range envelope.Data.Currencies {
		rates[currency.Name] = currency.Rate
	}

	fmt.Printf(
		"%s %.2f = %s %.2f (on %s)\n",
		*from, *n, *to, *n*currencyRate(*to)/currencyRate(*from), envelope.Data.Date,
	)
}
```

Try it out! If this is your first time compiling, it may take a while to
build the standard library. Depending on how frequently you run garbage
collection, it may be helpful to create garbage collector roots for
`pkgs.goPackages.std.export` and `pkgs.goPackages.std.lib`.

```console
$ nix run . -- -f INR -t ISK -n 150
INR 150.00 = ISK 223.98 (on 2025-05-26)
````

It would be nice to have the currencies formatted in their local formatting.
This can be done with the `golang.org/x/text/currency` package. While that
package doesn't have any external dependencies, it does use quite a few internal
modules which all have to be manually specified so Nix knows the correct order
to build them in. These lists get quite long so I prefer to keep them in a
separate file. Create a new file called `third-party.nix`: 

<details>
<summary>Show contents of third-party.nix</summary>

<br />

```nix
{ buildGoLibrary, fetchgit }:

let
  src = src: prefix: builtins.map (path: "${srcs."${src}"}/${prefix}/${path}");

  srcs = {
    "golang.org/x/text" = fetchgit {
      url = "https://go.googlesource.com/text";
      rev = "700cc20645cf719b928f5fce7e07528c4f7fa601";
      hash = "sha256-jibU2+7j+NLUORjR5uGMjn88BAbCPZWQX05fQKsYJeU=";
    };
  };

  goPackages = {
    "golang.org/x/text/currency" = buildGoLibrary {
      packagePath = "golang.org/x/text/currency";
      srcs = src "golang.org/x/text" "currency" [
        "common.go"
        "currency.go"
        "format.go"
        "query.go"
        "tables.go"
      ];
      imports = [
        goPackages."golang.org/x/text/internal/format"
        goPackages."golang.org/x/text/internal/language/compact"
        goPackages."golang.org/x/text/internal/number"
        goPackages."golang.org/x/text/internal/tag"
        goPackages."golang.org/x/text/language"
      ];
    };

    "golang.org/x/text/internal/format" = buildGoLibrary {
      packagePath = "golang.org/x/text/internal/format";
      srcs = src "golang.org/x/text" "internal/format" [
        "format.go"
        "parser.go"
      ];
      imports = [ goPackages."golang.org/x/text/language" ];
    };

    "golang.org/x/text/internal/language" = buildGoLibrary {
      packagePath = "golang.org/x/text/internal/language";
      srcs = src "golang.org/x/text" "internal/language" [
        "common.go"
        "compact.go"
        "compose.go"
        "coverage.go"
        "language.go"
        "lookup.go"
        "match.go"
        "parse.go"
        "tables.go"
        "tags.go"
      ];
      imports = [ goPackages."golang.org/x/text/internal/tag" ];
    };

    "golang.org/x/text/internal/language/compact" = buildGoLibrary {
      packagePath = "golang.org/x/text/internal/language/compact";
      srcs = src "golang.org/x/text" "internal/language/compact" [
        "compact.go"
        "language.go"
        "parents.go"
        "tables.go"
        "tags.go"
      ];
      imports = [ goPackages."golang.org/x/text/internal/language" ];
    };

    "golang.org/x/text/internal/number" = buildGoLibrary {
      packagePath = "golang.org/x/text/internal/number";
      srcs = src "golang.org/x/text" "internal/number" [
        "common.go"
        "decimal.go"
        "format.go"
        "number.go"
        "pattern.go"
        "roundingmode_string.go"
        "tables.go"
      ];
      imports = [
        goPackages."golang.org/x/text/internal/language/compact"
        goPackages."golang.org/x/text/internal/stringset"
        goPackages."golang.org/x/text/language"
      ];
    };

    "golang.org/x/text/internal/stringset" = buildGoLibrary {
      packagePath = "golang.org/x/text/internal/stringset";
      srcs = src "golang.org/x/text" "internal/stringset" [ "set.go" ];
    };

    "golang.org/x/text/internal/tag" = buildGoLibrary {
      packagePath = "golang.org/x/text/internal/tag";
      srcs = src "golang.org/x/text" "internal/tag" [ "tag.go" ];
    };

    "golang.org/x/text/language" = buildGoLibrary {
      packagePath = "golang.org/x/text/language";
      srcs = src "golang.org/x/text" "language" [
        "coverage.go"
        "doc.go"
        "language.go"
        "match.go"
        "parse.go"
        "tables.go"
        "tags.go"
      ];
      imports = [
        goPackages."golang.org/x/text/internal/language"
        goPackages."golang.org/x/text/internal/language/compact"
      ];
    };
  };

in
goPackages
```

</details>

Now with our dependencies set up, we can import this into our flake and register
the currency package as an import of our app.

```diff
           pkgs = import nixpkgs {
             inherit system;
             overlays = [ inputs.gopkg2nix-incremental.overlays.default ];
           };
+
+          thirdParty = pkgs.callPackage ./third-party.nix {};
 
         in
         {
           packages."${system}" = {
             default = pkgs.buildGoBinary {
               name = "currency";
               srcs = [ ./currency.go ];
+              imports = [ thirdParty."golang.org/x/text/currency" ];
             };
           };
         }
```

Finally, the package can be used in our program's code.

```diff
 	"fmt"
 	"log"
 	"net/http"
+
+	"golang.org/x/text/currency"
 )
 
 const source = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
...
 		rates[currency.Name] = currency.Rate
 	}
 
+	fromRate, fromUnit := currencyRate(*from), currency.MustParseISO(*from)
+	toRate, toUnit := currencyRate(*to), currency.MustParseISO(*to)
+	fromAmount, toAmount := fromUnit.Amount(*n), toUnit.Amount(*n*toRate/fromRate)
 
 	fmt.Printf(
-		"%s %.2f = %s %.2f (on %s)\n",
-		*from, *n, *to, *n*currencyRate(*to)/currencyRate(*from), envelope.Data.Date,
+		"%v = %v (on %s)\n",
+		currency.Symbol(fromAmount), currency.Symbol(toAmount), envelope.Data.Date,
 	)
 }
```

And our app now returns values with nice formatting!

```console
$ nix run . -- -f INR -t ISK -n 150
₹ 150.00 = ISK 224 (on 2025-05-26)
````

</details>

<br />

#### License

<sup>
Copyright (C) jae beller, 2025.
</sup>
<br />
<sup>
Released under the <a href="https://www.gnu.org/licenses/lgpl-3.0.txt">GNU Lesser General Public License, Version 3.0</a> or later. See <a href="LICENSE">LICENSE</a> for more information.
</sup>
