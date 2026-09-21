// Copyright IBM Corp. 2025
// SPDX-License-Identifier: MPL-2.0

package tools

func ptr[T any](v T) *T {
	return &v
}
