package tools

import (
    "net/http"
    "strings"
    "testing"

    log "github.com/sirupsen/logrus"
    "github.com/hashicorp/terraform-mcp-server/pkg/client"
)

func TestSearchProvidersPrioritizesOfficial(t *testing.T) {
    // Backup original and restore
    original := sendRegistryCall
    defer func() { sendRegistryCall = original }()

    // Fake responses
    sendRegistryCall = func(httpClient *http.Client, method string, uri string, logger *log.Logger, callOptions ...string) ([]byte, error) {
        // provider list call
        if strings.HasPrefix(uri, "providers?filter[name]=") {
            // Return two providers: community then official (unordered)
            // minimal JSON with attributes name, namespace, tier
            return []byte(`{"data":[{"id":"1","attributes":{"name":"keycloak","namespace":"mrparkers","tier":"community"}},{"id":"2","attributes":{"name":"keycloak","namespace":"keycloak-official","tier":"official"}}]}`), nil
        }

        // provider docs calls: uri like providers/{namespace}/{name}/{version}
        if strings.HasPrefix(uri, "providers/mrparkers/") {
            return []byte(`{"docs":[{"id":"doc1","title":"Keycloak (community)","path":"","slug":"keycloak","category":"resources","language":"hcl"}]}`), nil
        }
        if strings.HasPrefix(uri, "providers/keycloak-official/") {
            return []byte(`{"docs":[{"id":"doc2","title":"Keycloak (official)","path":"","slug":"keycloak","category":"resources","language":"hcl"}]}`), nil
        }

        return nil, nil
    }

    logger := log.New()
    providerDetail := client.ProviderDetail{
        ProviderName:      "keycloak",
        ProviderNamespace: "",
        ProviderVersion:   "latest",
        ProviderDataType:  "resources",
    }

    result, err := searchProvidersDocs(http.DefaultClient, providerDetail, "keycloak", "default guide", logger)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // official provider should appear before community provider in the output
    officialIdx := strings.Index(result, "Provider: keycloak-official/keycloak (Tier: official)")
    communityIdx := strings.Index(result, "Provider: mrparkers/keycloak (Tier: community)")
    if officialIdx == -1 || communityIdx == -1 {
        t.Fatalf("expected both providers in result, got: %s", result)
    }
    if officialIdx > communityIdx {
        t.Fatalf("official provider found after community provider; result: %s", result)
    }
}
