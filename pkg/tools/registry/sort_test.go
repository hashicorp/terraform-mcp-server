package tools

import (
	"testing"
)

func TestSortMatchesByTier(t *testing.T) {
	matches := []providerMatch{
		{Namespace: "a", Name: "one", Tier: "community"},
		{Namespace: "b", Name: "two", Tier: "partner"},
		{Namespace: "c", Name: "three", Tier: "official"},
	}

	sortMatchesByTier(matches)

	if matches[0].Tier != "official" || matches[1].Tier != "partner" || matches[2].Tier != "community" {
		t.Fatalf("unexpected tier order: %v", []string{matches[0].Tier, matches[1].Tier, matches[2].Tier})
	}
}
