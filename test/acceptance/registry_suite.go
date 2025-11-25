package acceptance

import "regexp"

var RegistryToolTests = []ToolAcceptanceTest{
	{
		Name:        "get_latest_provider_version_nonexistent",
		Description: "Get latest provider version for a provider that doesn't exist",
		ToolName:    "get_latest_provider_version",
		Arguments: map[string]interface{}{
			"namespace": "initech",
			"name":      "y2kbanksoftware",
		},
		ExpectError: regexp.MustCompile(`404 Not Found`),
	},
	{
		Name:        "get_latest_provider_version",
		Description: "Get latest provider version for a well known provider",
		ToolName:    "get_latest_provider_version",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "aws",
		},
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
	},
}
