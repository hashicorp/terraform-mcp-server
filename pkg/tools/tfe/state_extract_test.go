// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package tools

import (
	"testing"
)

func TestBuildResourceAddress(t *testing.T) {
	cases := []struct {
		name string
		res  stateResource
		inst stateInstance
		want string
	}{
		{
			name: "simple managed resource",
			res:  stateResource{Type: "aws_s3_bucket", Name: "assets", Mode: "managed"},
			inst: stateInstance{},
			want: "aws_s3_bucket.assets",
		},
		{
			name: "data source",
			res:  stateResource{Type: "aws_ami", Name: "ubuntu", Mode: "data"},
			inst: stateInstance{},
			want: "data.aws_ami.ubuntu",
		},
		{
			name: "in a module",
			res:  stateResource{Type: "aws_vpc", Name: "main", Mode: "managed", Module: "module.network"},
			inst: stateInstance{},
			want: "module.network.aws_vpc.main",
		},
		{
			name: "for_each string key",
			res:  stateResource{Type: "aws_iam_role", Name: "svc", Mode: "managed"},
			inst: stateInstance{IndexKey: "reader"},
			want: `aws_iam_role.svc["reader"]`,
		},
		{
			name: "count numeric index",
			res:  stateResource{Type: "aws_instance", Name: "web", Mode: "managed"},
			inst: stateInstance{IndexKey: float64(2)},
			want: "aws_instance.web[2]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildResourceAddress(tc.res, tc.inst)
			if got != tc.want {
				t.Errorf("buildResourceAddress() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractResourcesDefaultsRootModule(t *testing.T) {
	state := parseTestState(t, `{
		"version": 4,
		"resources": [{
			"mode": "managed", "type": "aws_vpc", "name": "main", "provider": "p",
			"instances": [{"attributes": {"id": "vpc-1"}}]
		}]
	}`)

	resources := extractResources(state)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0].Module != "(root)" {
		t.Errorf("expected root module resource to be labeled \"(root)\", got %q", resources[0].Module)
	}
}

func TestStateMaxSizeBytesDefaultAndOverride(t *testing.T) {
	t.Setenv("TF_STATE_MAX_SIZE_MB", "")
	if got, want := stateMaxSizeBytes(), int64(defaultStateMaxSizeMB)*1024*1024; got != want {
		t.Errorf("default stateMaxSizeBytes() = %d, want %d", got, want)
	}

	t.Setenv("TF_STATE_MAX_SIZE_MB", "5")
	if got, want := stateMaxSizeBytes(), int64(5)*1024*1024; got != want {
		t.Errorf("overridden stateMaxSizeBytes() = %d, want %d", got, want)
	}

	// Invalid values fall back to the default rather than erroring.
	t.Setenv("TF_STATE_MAX_SIZE_MB", "not-a-number")
	if got, want := stateMaxSizeBytes(), int64(defaultStateMaxSizeMB)*1024*1024; got != want {
		t.Errorf("invalid-env stateMaxSizeBytes() = %d, want %d", got, want)
	}
}
