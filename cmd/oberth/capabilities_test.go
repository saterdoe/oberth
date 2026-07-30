package main

import (
	"strings"
	"testing"
)

func TestFormatCapabilitiesShowsMissingProviderHonestly(t *testing.T) {
	output := formatCapabilities(capabilities{
		SchemaVersion: "1",
		Skills:        []capability{{Name: "Transactional workspace", Status: "active", Description: "isolated"}},
	})
	for _, expected := range []string{"Capabilities schema 1", "Providers\n  (none)", "[active] Transactional workspace"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected %q in output:\n%s", expected, output)
		}
	}
}
