// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package orchestrator

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// ConfigModifier handles parsing and modification of Terraform configuration files
type ConfigModifier struct {
	parser *hclparse.Parser
}

// NewConfigModifier creates a new configuration modifier instance
func NewConfigModifier() *ConfigModifier {
	return &ConfigModifier{
		parser: hclparse.NewParser(),
	}
}

// ParseTerraformConfig parses base64 encoded tar.gz content and extracts Terraform configuration
func (cm *ConfigModifier) ParseTerraformConfig(content []byte) (*TerraformConfig, error) {
	// Decode base64 content
	decodedContent, err := base64.StdEncoding.DecodeString(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 content: %w", err)
	}

	// Extract tar.gz content
	files, err := cm.extractTarGz(decodedContent)
	if err != nil {
		return nil, fmt.Errorf("failed to extract tar.gz: %w", err)
	}

	// Initialize configuration
	config := &TerraformConfig{
		ProviderBlocks: make(map[string]*ProviderBlock),
		ResourceBlocks: make(map[string]*ResourceBlock),
		Variables:      make(map[string]*Variable),
		Outputs:        make(map[string]*Output),
		Locals:         make(map[string]*Local),
		ModuleCalls:    make(map[string]*ModuleCall),
		DataSources:    make(map[string]*DataSource),
		Files:          files,
	}

	// Parse each Terraform file
	for filename, fileContent := range files {
		if cm.isTerraformFile(filename) {
			if err := cm.parseHCLFile(filename, fileContent, config); err != nil {
				return nil, fmt.Errorf("failed to parse file %s: %w", filename, err)
			}
		}
	}

	return config, nil
}

// AddDefaultTags adds default tags to the configuration
func (cm *ConfigModifier) AddDefaultTags(config *TerraformConfig, tags map[string]string) error {
	// Add tags to provider blocks
	for _, provider := range config.ProviderBlocks {
		if provider.DefaultTags == nil {
			provider.DefaultTags = make(map[string]string)
		}
		for k, v := range tags {
			provider.DefaultTags[k] = v
		}
	}

	return nil
}

// UpdateProviderConfigurations updates provider configurations
func (cm *ConfigModifier) UpdateProviderConfigurations(config *TerraformConfig, updates map[string]interface{}) error {
	for providerName, updateConfig := range updates {
		if provider, exists := config.ProviderBlocks[providerName]; exists {
			if configMap, ok := updateConfig.(map[string]interface{}); ok {
				for k, v := range configMap {
					provider.Configuration[k] = v
				}
			}
		}
	}

	return nil
}

// SerializeConfig serializes the configuration back to base64 encoded tar.gz
func (cm *ConfigModifier) SerializeConfig(config *TerraformConfig) ([]byte, error) {
	// Create a buffer for the tar.gz content
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	// Write files back to tar
	for filename, content := range config.Files {
		header := &tar.Header{
			Name: filename,
			Size: int64(len(content)),
			Mode: 0644,
		}

		if err := tarWriter.WriteHeader(header); err != nil {
			return nil, fmt.Errorf("failed to write tar header: %w", err)
		}

		if _, err := tarWriter.Write(content); err != nil {
			return nil, fmt.Errorf("failed to write file content: %w", err)
		}
	}

	// Close writers
	if err := tarWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err := gzipWriter.Close(); err != nil {
		return nil, fmt.Errorf("failed to close gzip writer: %w", err)
	}

	// Encode to base64
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return []byte(encoded), nil
}

// extractTarGz extracts tar.gz content to a map of filename -> content
func (cm *ConfigModifier) extractTarGz(data []byte) (map[string][]byte, error) {
	files := make(map[string][]byte)

	gzipReader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		if header.Typeflag == tar.TypeReg {
			content, err := io.ReadAll(tarReader)
			if err != nil {
				return nil, fmt.Errorf("failed to read file content: %w", err)
			}
			files[header.Name] = content
		}
	}

	return files, nil
}

// isTerraformFile checks if a file is a Terraform configuration file
func (cm *ConfigModifier) isTerraformFile(filename string) bool {
	return strings.HasSuffix(filename, ".tf") || strings.HasSuffix(filename, ".tf.json")
}

// parseHCLFile parses a single HCL file and extracts components
func (cm *ConfigModifier) parseHCLFile(filename string, content []byte, config *TerraformConfig) error {
	// Parse the HCL file
	_, diags := cm.parser.ParseHCL(content, filename)
	if diags.HasErrors() {
		return fmt.Errorf("HCL parse errors: %s", diags.Error())
	}

	// For now, create a sample provider block if the file contains provider configuration
	if strings.Contains(string(content), "provider") {
		provider := &ProviderBlock{
			Name:          "aws",
			Configuration: make(map[string]interface{}),
			DefaultTags:   make(map[string]string),
			FileName:      filename,
			LineNumber:    1,
		}
		config.ProviderBlocks["aws"] = provider
	}

	return nil
}
