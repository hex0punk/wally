package internal

import (
	"fmt"
	"go/token"
	"golang.org/x/tools/go/packages"
	"os"
)

func LoadPackages(path string) []*packages.Package {
	fset := token.NewFileSet()

	cfg := &packages.Config{
		Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedExportFile | packages.NeedTypesSizes | packages.NeedModule | packages.NeedDeps,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}

	return pkgs
}

func LoadPackagesForAnalyzer(path string) []*packages.Package {
	fset := token.NewFileSet()

	cfg := &packages.Config{
		Mode: packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo |
			packages.NeedName | packages.NeedCompiledGoFiles | packages.NeedImports |
			packages.NeedExportFile | packages.NeedTypesSizes | packages.NeedModule | packages.NeedDeps,
		Fset: fset,
	}

	pkgs, err := packages.Load(cfg, path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load: %v\n", err)
		os.Exit(1)
	}

	return pkgs
}
