// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package toolsets

// Add search tools without moving or duplicating the registry and Terraform
// tool definitions maintained in registry.go.
func init() {
	AllTools = append(AllTools, searchTools...)
}
