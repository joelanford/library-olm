//go:build ignore

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	jsonnet "github.com/google/go-jsonnet"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"sigs.k8s.io/yaml"
)

type BundleMetadata struct {
	Format  string `yaml:"format"`
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
	Release string `yaml:"release,omitempty"`
}

func main() {
	configFile := flag.String("config", "", "path to JSON config file (overrides bundle defaults)")
	flag.Parse()

	if flag.NArg() != 1 {
		log.Fatal("usage: render [-config config.json] <bundle-dir>")
	}
	bundleDir := flag.Arg(0)

	config := map[string]any{}
	if *configFile != "" {
		data, err := os.ReadFile(*configFile)
		if err != nil {
			log.Fatalf("read config: %v", err)
		}
		if err := json.Unmarshal(data, &config); err != nil {
			log.Fatalf("parse config: %v", err)
		}
	}

	if err := run(bundleDir, config); err != nil {
		log.Fatal(err)
	}
}

func run(bundleDir string, config map[string]any) error {
	// Parse bundle.yaml
	bundleYAML, err := os.ReadFile(filepath.Join(bundleDir, "bundle.yaml"))
	if err != nil {
		return fmt.Errorf("read bundle.yaml: %w", err)
	}
	var meta BundleMetadata
	if err := yaml.Unmarshal(bundleYAML, &meta); err != nil {
		return fmt.Errorf("parse bundle.yaml: %w", err)
	}
	if meta.Format != "registry+v2" {
		return fmt.Errorf("unsupported format: %s", meta.Format)
	}

	// Validate user-provided config against schema
	schemaPath := filepath.Join(bundleDir, "config.schema.json")
	if _, err := os.Stat(schemaPath); err == nil {
		if err := validateConfig(schemaPath, config); err != nil {
			return fmt.Errorf("config validation: %w", err)
		}
	}

	// Evaluate Jsonnet with bundle and config as top-level arguments
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	bundleJSON, err := json.Marshal(map[string]string{
		"name":    meta.Name,
		"version": meta.Version,
		"release": meta.Release,
	})
	if err != nil {
		return fmt.Errorf("marshal bundle metadata: %w", err)
	}

	resourcesDir := filepath.Join(bundleDir, "resources")
	vm := jsonnet.MakeVM()
	vm.TLACode("bundle", string(bundleJSON))
	vm.TLACode("config", string(configJSON))

	entrypoint := filepath.Join(resourcesDir, "main.jsonnet")
	output, err := vm.EvaluateFile(entrypoint)
	if err != nil {
		return fmt.Errorf("jsonnet: %w", err)
	}

	yamlOutput, err := yaml.JSONToYAML([]byte(output))
	if err != nil {
		return fmt.Errorf("yaml to json: %w", err)
	}

	fmt.Print(string(yamlOutput))
	return nil
}

func validateConfig(schemaPath string, config map[string]any) error {
	c := jsonschema.NewCompiler()
	s, err := c.Compile(schemaPath)
	if err != nil {
		return fmt.Errorf("compile schema: %w", err)
	}
	if err := s.Validate(config); err != nil {
		return err
	}
	return nil
}
