package acceptance

import "regexp"

var RegistryToolTests = []ToolAcceptanceTest{
	{
		Description: "Get latest provider version for a provider that doesn't exist",
		ToolName:    "get_latest_provider_version",
		Arguments: map[string]interface{}{
			"namespace": "initech",
			"name":      "y2kbanksoftware",
		},
		ExpectError: regexp.MustCompile(`404 Not Found`),
	},
	{
		Description: "Get latest provider version for a provider that doesn't exist",
		ToolName:    "get_latest_provider_version",
		Arguments: map[string]interface{}{
			"namespace": "hashicorp",
			"name":      "aws",
		},
		ExpectTextContent: regexp.MustCompile(`^\d+\.\d+\.\d+$`),
	},
}
