package onek

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/1homsi/onekit/internal/gengo"
	"github.com/1homsi/onekit/internal/genopenapi"
	"github.com/1homsi/onekit/internal/genpy"
	"github.com/1homsi/onekit/internal/gents"
	"github.com/1homsi/onekit/internal/onkcompile"
	"github.com/1homsi/onekit/internal/onkir"
	"github.com/1homsi/onekit/internal/onklang"
)

func discoverOnkFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".onk") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", dir, err)
	}
	return files, nil
}

func parseSources(paths []string) ([]onkcompile.Source, error) {
	var sources []onkcompile.Source
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		ast, err := onklang.Parse(string(data))
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		sources = append(sources, onkcompile.Source{Path: path, AST: ast})
	}
	return sources, nil
}

// mergeFiles combines every compiled onkir.File in a package into a single
// File, since a project's .onk sources typically target one generated Go
// package (or one TS/Python module) per output directory regardless of how
// many onk `package` declarations they're split across.
func mergeFiles(pkg *onkir.Package, goPackage string) *onkir.File {
	merged := &onkir.File{Package: goPackage}
	for _, f := range pkg.Files {
		merged.Messages = append(merged.Messages, f.Messages...)
		merged.Enums = append(merged.Enums, f.Enums...)
		merged.Services = append(merged.Services, f.Services...)
	}
	for _, m := range merged.Messages {
		m.File = merged
	}
	for _, e := range merged.Enums {
		e.File = merged
	}
	for _, s := range merged.Services {
		s.File = merged
	}
	return merged
}

// Check parses and compiles every .onk file under dir without generating
// any output, returning the first error encountered.
func Check(dir string) error {
	files, err := discoverOnkFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no .onk files found under %s", dir)
	}
	sources, err := parseSources(files)
	if err != nil {
		return err
	}
	_, err = onkcompile.Compile(sources)
	return err
}

const (
	genDirPerm  = 0o750
	genFilePerm = 0o600
)

func writeFile(path string, data []byte) error {
	if len(data) == 0 {
		return nil
	}
	err := os.MkdirAll(filepath.Dir(path), genDirPerm)
	if err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	err = os.WriteFile(path, data, genFilePerm)
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func lastPathSegment(p string) string {
	p = strings.TrimSuffix(p, "/")
	parts := strings.Split(filepath.ToSlash(p), "/")
	return parts[len(parts)-1]
}

// Build parses and compiles every .onk file under dir, then generates every
// target configured in onekit.toml.
func Build(dir string) error {
	cfg, err := LoadConfig(dir)
	if err != nil {
		return err
	}

	files, err := discoverOnkFiles(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no .onk files found under %s", dir)
	}
	sources, err := parseSources(files)
	if err != nil {
		return err
	}
	pkg, err := onkcompile.Compile(sources)
	if err != nil {
		return err
	}

	steps := []struct {
		enabled bool
		run     func() error
	}{
		{cfg.Generate.GoServer != nil || cfg.Generate.GoClient != nil, func() error { return buildGo(cfg, pkg) }},
		{cfg.Generate.TSClient != nil, func() error { return buildTSClient(cfg, pkg) }},
		{cfg.Generate.TSServer != nil, func() error { return buildTSServer(cfg, pkg) }},
		{cfg.Generate.PythonClient != nil, func() error { return buildPythonClient(cfg, pkg) }},
		{cfg.Generate.OpenAPI != nil, func() error { return buildOpenAPI(cfg, pkg) }},
	}
	for _, step := range steps {
		if !step.enabled {
			continue
		}
		err = step.run()
		if err != nil {
			return err
		}
	}
	return nil
}

func buildGo(cfg *Config, pkg *onkir.Package) error {
	out := cfg.Generate.GoServer
	if out == nil {
		out = cfg.Generate.GoClient
	}
	outDir := cfg.resolve(out.Out)
	goPackage := lastPathSegment(outDir)
	merged := mergeFiles(pkg, goPackage)

	err := writeGoTypesAndValidation(merged, outDir)
	if err != nil {
		return err
	}
	if cfg.Generate.GoServer != nil {
		err = writeGoServer(merged, cfg.resolve(cfg.Generate.GoServer.Out))
		if err != nil {
			return err
		}
	}
	if cfg.Generate.GoClient != nil {
		err = writeGoClient(merged, cfg.resolve(cfg.Generate.GoClient.Out))
		if err != nil {
			return err
		}
	}
	return nil
}

func writeGoTypesAndValidation(merged *onkir.File, outDir string) error {
	types, err := gengo.GenerateTypes(merged)
	if err != nil {
		return fmt.Errorf("generate go types: %w", err)
	}
	err = writeFile(filepath.Join(outDir, "types.gen.go"), types)
	if err != nil {
		return err
	}

	validation, err := gengo.GenerateValidation(merged)
	if err != nil {
		return fmt.Errorf("generate go validation: %w", err)
	}
	return writeFile(filepath.Join(outDir, "validate.gen.go"), validation)
}

func writeGoServer(merged *onkir.File, outDir string) error {
	server, err := gengo.GenerateServer(merged)
	if err != nil {
		return fmt.Errorf("generate go server: %w", err)
	}
	return writeFile(filepath.Join(outDir, "server.gen.go"), server)
}

func writeGoClient(merged *onkir.File, outDir string) error {
	client, err := gengo.GenerateClient(merged)
	if err != nil {
		return fmt.Errorf("generate go client: %w", err)
	}
	return writeFile(filepath.Join(outDir, "client.gen.go"), client)
}

func buildTSClient(cfg *Config, pkg *onkir.Package) error {
	outDir := cfg.resolve(cfg.Generate.TSClient.Out)
	merged := mergeFiles(pkg, "")
	err := writeFile(filepath.Join(outDir, "types.ts"), gents.GenerateTypes(merged))
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "client.ts"), gents.GenerateClient(merged))
}

func buildTSServer(cfg *Config, pkg *onkir.Package) error {
	outDir := cfg.resolve(cfg.Generate.TSServer.Out)
	merged := mergeFiles(pkg, "")
	err := writeFile(filepath.Join(outDir, "types.ts"), gents.GenerateTypes(merged))
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "server.ts"), gents.GenerateServer(merged))
}

func buildPythonClient(cfg *Config, pkg *onkir.Package) error {
	outDir := cfg.resolve(cfg.Generate.PythonClient.Out)
	merged := mergeFiles(pkg, "")
	err := writeFile(filepath.Join(outDir, "models.py"), genpy.GenerateTypes(merged))
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(outDir, "client.py"), genpy.GenerateClient(merged, "models"))
}

func buildOpenAPI(cfg *Config, pkg *onkir.Package) error {
	outDir := cfg.resolve(cfg.Generate.OpenAPI.Out)
	merged := mergeFiles(pkg, "")
	opts := genopenapi.Options{
		Title:       cfg.Generate.OpenAPI.Title,
		Version:     cfg.Generate.OpenAPI.Version,
		Description: cfg.Generate.OpenAPI.Description,
	}
	data, err := genopenapi.Generate(merged, opts)
	if err != nil {
		return fmt.Errorf("generate openapi: %w", err)
	}
	return writeFile(filepath.Join(outDir, "openapi.yaml"), data)
}
