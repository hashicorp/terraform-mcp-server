// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package client

import "time"

type ProviderDetail struct {
	ProviderName      string
	ProviderNamespace string
	ProviderVersion   string
	ProviderDataType  string
}

type ModuleDetail struct {
	ModuleName      string
	ModuleNamespace string
	ModuleProvider  string
}

// TerraformModule represents the structure of a Terraform module list response.
// Note: The API seems to return different structures, this one matches the
// format where the top-level key is "modules".
type TerraformModules struct {
	Metadata struct {
		Limit         int    `json:"limit"`          // Limit is 15
		CurrentOffset int    `json:"current_offset"` // always starts at 0
		NextOffset    int    `json:"next_offset"`    // always starts at 15
		PrevOffset    int    `json:"prev_offset"`    // always starts at nil
		NextURL       string `json:"next_url"`
		PrevURL       string `json:"prev_url"`
	} `json:"meta"`
	Data []struct {
		ID          string    `json:"id"`
		Owner       string    `json:"owner"`
		Namespace   string    `json:"namespace"`
		Name        string    `json:"name"`
		Version     string    `json:"version"`
		Provider    string    `json:"provider"`
		Description string    `json:"description"`
		Source      string    `json:"source"`
		Tag         string    `json:"tag"`
		PublishedAt time.Time `json:"published_at"`
		Downloads   int64     `json:"downloads"`
		Verified    bool      `json:"verified"`
	} `json:"modules"`
}

// ModuleInput represents a Terraform module input variable.
type ModuleInput struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Default     any    `json:"default"` // Can be string, bool, number, etc.
	Required    bool   `json:"required"`
}

// ModuleOutput represents a Terraform module output value.
type ModuleOutput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ModuleDependency represents a Terraform module dependency.
type ModuleDependency struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Version string `json:"version"`
}

// ModuleProviderDependency represents a Terraform provider dependency.
type ModuleProviderDependency struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Source    string `json:"source"`
	Version   string `json:"version"`
}

// ModuleResource represents a resource within a Terraform module.
type ModuleResource struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ModulePart represents the structure of the root, submodules, or examples
// within a Terraform module version details response.
type ModulePart struct {
	Path                 string                     `json:"path"`
	Name                 string                     `json:"name"`
	Readme               string                     `json:"readme"`
	Empty                bool                       `json:"empty"`
	Inputs               []ModuleInput              `json:"inputs"`
	Outputs              []ModuleOutput             `json:"outputs"`
	Dependencies         []ModuleDependency         `json:"dependencies"`
	ProviderDependencies []ModuleProviderDependency `json:"provider_dependencies"`
	Resources            []ModuleResource           `json:"resources"`
}

// TerraformModuleVersionDetails represents the detailed structure of a specific
// Terraform module version response.
type TerraformModuleVersionDetails struct {
	ID              string       `json:"id"`
	Owner           string       `json:"owner"`
	Namespace       string       `json:"namespace"`
	Name            string       `json:"name"`
	Version         string       `json:"version"`
	Provider        string       `json:"provider"`
	ProviderLogoURL string       `json:"provider_logo_url"`
	Description     string       `json:"description"`
	Source          string       `json:"source"`
	Tag             string       `json:"tag"`
	PublishedAt     time.Time    `json:"published_at"`
	Downloads       int64        `json:"downloads"`
	Verified        bool         `json:"verified"`
	Root            ModulePart   `json:"root"`
	Submodules      []ModulePart `json:"submodules"`
	Examples        []ModulePart `json:"examples"`
	Providers       []string     `json:"providers"`
	Versions        []string     `json:"versions"`
	Deprecation     any          `json:"deprecation"` // Assuming it can be null or an object
}

// ProviderLatest represents the structure of the latest provider response.
// https://registry.terraform.io/v1/providers/hashicorp/consul/latest
type ProviderVersionLatest struct {
	ID          string    `json:"id"`
	Owner       string    `json:"owner"`
	Namespace   string    `json:"namespace"`
	Name        string    `json:"name"`
	Alias       string    `json:"alias"`
	Version     string    `json:"version"`
	Tag         string    `json:"tag"`
	Description string    `json:"description"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
	Downloads   int64     `json:"downloads"`
	Tier        string    `json:"tier"`
	LogoURL     string    `json:"logo_url"`
	Versions    []string  `json:"versions"`
}

// ProviderDoc represents a single documentation item.
type ProviderDoc struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Path        string `json:"path"`
	Slug        string `json:"slug"`
	Category    string `json:"category"`
	Subcategory string `json:"subcategory"`
	Language    string `json:"language"`
}

// ProviderDocs represents the structure of the provider details response.
type ProviderDocs struct {
	ID          string        `json:"id"`
	Owner       string        `json:"owner"`
	Namespace   string        `json:"namespace"`
	Name        string        `json:"name"`
	Alias       string        `json:"alias"`
	Version     string        `json:"version"`
	Tag         string        `json:"tag"`
	Description string        `json:"description"`
	Source      string        `json:"source"`
	PublishedAt string        `json:"published_at"`
	Downloads   int64         `json:"downloads"`
	Tier        string        `json:"tier"`
	LogoURL     string        `json:"logo_url"`
	Versions    []string      `json:"versions"`
	Docs        []ProviderDoc `json:"docs"`
}

// ProviderList represents the structure of the provider list response.
// https://registry.terraform.io/v2/providers?filter[tier]=official
type ProviderList struct {
	Data []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Alias         string `json:"alias"`
			Description   string `json:"description"`
			Downloads     int    `json:"downloads"`
			Featured      bool   `json:"featured"`
			FullName      string `json:"full-name"`
			LogoURL       string `json:"logo-url"`
			Name          string `json:"name"`
			Namespace     string `json:"namespace"`
			OwnerName     string `json:"owner-name"`
			RobotsNoindex bool   `json:"robots-noindex"`
			Source        string `json:"source"`
			Tier          string `json:"tier"`
			Unlisted      bool   `json:"unlisted"`
			Warning       string `json:"warning"`
		} `json:"attributes"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"data"`
	Links struct {
		First string `json:"first"`
		Last  string `json:"last"`
		Next  string `json:"next"`
		Prev  any    `json:"prev"`
	} `json:"links"`
	Meta struct {
		Pagination struct {
			PageSize    int `json:"page-size"`
			CurrentPage int `json:"current-page"`
			NextPage    int `json:"next-page"`
			PrevPage    any `json:"prev-page"`
			TotalPages  int `json:"total-pages"`
			TotalCount  int `json:"total-count"`
		} `json:"pagination"`
	} `json:"meta"`
}

// ProviderVersion represents structure with list of provider versions.
type ProviderVersionList struct {
	Data struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Alias         string `json:"alias"`
			Description   string `json:"description"`
			Downloads     int64  `json:"downloads"`
			Featured      bool   `json:"featured"`
			FullName      string `json:"full-name"`
			LogoURL       string `json:"logo-url"`
			Name          string `json:"name"`
			Namespace     string `json:"namespace"`
			OwnerName     string `json:"owner-name"`
			RobotsNoindex bool   `json:"robots-noindex"`
			Source        string `json:"source"`
			Tier          string `json:"tier"`
			Unlisted      bool   `json:"unlisted"`
			Warning       string `json:"warning"`
		} `json:"attributes"`
		Relationships struct {
			ProviderVersions struct {
				Data []struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"data"`
				Links struct {
					Related string `json:"related"`
				} `json:"links"`
			} `json:"provider-versions"`
		} `json:"relationships"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"data"`
	Included []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Description string    `json:"description"`
			Downloads   int       `json:"downloads"`
			PublishedAt time.Time `json:"published-at"`
			Tag         string    `json:"tag"`
			Version     string    `json:"version"`
		} `json:"attributes"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"included"`
}

// ProviderResourceDetails represents the structure of the provider resource details response.
// https://registry.terraform.io/v2/provider-docs/8814952
type ProviderResourceDetails struct {
	Data struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Category    string `json:"category"`
			Content     string `json:"content"`
			Language    string `json:"language"`
			Path        string `json:"path"`
			Slug        string `json:"slug"`
			Subcategory string `json:"subcategory"`
			Title       string `json:"title"`
			Truncated   bool   `json:"truncated"`
		} `json:"attributes"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"data"`
}

// ProviderOverviewStruct represents the structure of the provider overview (how to use it) response.
// https://registry.terraform.io/v2/provider-docs?filter[provider-version]=70800&filter[category]=overview&filter[slug]=index
type ProviderOverviewStruct struct {
	Data []ProviderDocData `json:"data"`
}

type ProviderDocData struct {
	Type       string `json:"type"`
	ID         string `json:"id"`
	Attributes struct {
		Category    string      `json:"category"`
		Language    string      `json:"language"`
		Path        string      `json:"path"`
		Slug        string      `json:"slug"`
		Subcategory interface{} `json:"subcategory"`
		Title       string      `json:"title"`
		Truncated   bool        `json:"truncated"`
	} `json:"attributes"`
	Links struct {
		Self string `json:"self"`
	} `json:"links"`
}

// TerraformPolicyList represents the response structure for a list of Terraform policies
// retrieved from the HashiCorp Terraform Registry API.
// https://registry.terraform.io/v2/policies?page%5Bsize%5D=100&include=latest-version
type TerraformPolicyList struct {
	Data []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Downloads int    `json:"downloads"`
			FullName  string `json:"full-name"`
			Ingress   string `json:"ingress"`
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			OwnerName string `json:"owner-name"`
			Source    string `json:"source"`
			Title     string `json:"title"`
			Verified  bool   `json:"verified"`
		} `json:"attributes"`
		Relationships struct {
			LatestVersion struct {
				Data struct {
					ID   string `json:"id"`
					Type string `json:"type"`
				} `json:"data"`
				Links struct {
					Related string `json:"related"`
				} `json:"links"`
			} `json:"latest-version"`
		} `json:"relationships"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"data"`
	Included []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Description string    `json:"description"`
			Downloads   int       `json:"downloads"`
			PublishedAt time.Time `json:"published-at"`
			Readme      string    `json:"readme"`
			Source      string    `json:"source"`
			Tag         string    `json:"tag"`
			Version     string    `json:"version"`
		} `json:"attributes"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"included"`
	Links struct {
		First string `json:"first"`
		Last  string `json:"last"`
		Next  any    `json:"next"`
		Prev  any    `json:"prev"`
	} `json:"links"`
	Meta struct {
		Pagination struct {
			PageSize    int `json:"page-size"`
			CurrentPage int `json:"current-page"`
			NextPage    any `json:"next-page"`
			PrevPage    any `json:"prev-page"`
			TotalPages  int `json:"total-pages"`
			TotalCount  int `json:"total-count"`
		} `json:"pagination"`
	} `json:"meta"`
}

// TerraformPolicyDetails represents the detailed response structure for a Terraform policy
// as returned by the Terraform Registry API.
// https://registry.terraform.io/v2/policies/hashicorp/CIS-Policy-Set-for-AWS-Terraform/1.0.1?include=policies,policy-modules,policy-library
type TerraformPolicyDetails struct {
	Data struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Description string    `json:"description"`
			Downloads   int       `json:"downloads"`
			PublishedAt time.Time `json:"published-at"`
			Readme      string    `json:"readme"`
			Source      string    `json:"source"`
			Tag         string    `json:"tag"`
			Version     string    `json:"version"`
		} `json:"attributes"`
		Relationships struct {
			Policies struct {
				Data []struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"policies"`
			PolicyLibrary struct {
				Data struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"policy-library"`
			PolicyModules struct {
				Data []struct {
					Type string `json:"type"`
					ID   string `json:"id"`
				} `json:"data"`
			} `json:"policy-modules"`
		} `json:"relationships"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"data"`
	Included []struct {
		Type       string `json:"type"`
		ID         string `json:"id"`
		Attributes struct {
			Description string `json:"description"`
			Downloads   int    `json:"downloads"`
			FullName    string `json:"full-name"`
			Name        string `json:"name"`
			Shasum      string `json:"shasum"`
			ShasumType  string `json:"shasum-type"`
			Title       string `json:"title"`
		} `json:"attributes"`
		Links struct {
			Self string `json:"self"`
		} `json:"links"`
	} `json:"included"`
}
