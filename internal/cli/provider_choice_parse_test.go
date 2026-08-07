package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

func TestParseProviderChoices(t *testing.T) {
	resource := capabilitypack.ResourceIdentity{Kind: "skill", ID: "storage"}
	got, err := parseProviderChoices([]string{"cap:storage=provider/skill:storage", "cap:legacy=legacy"})
	if err != nil {
		t.Fatal(err)
	}
	want := []capabilitypack.ProviderChoice{
		{Capability: "cap:storage", ProviderPack: "provider", ProviderResource: &resource},
		{Capability: "cap:legacy", ProviderPack: "legacy"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider choices = %#v, want %#v", got, want)
	}
}

func TestParseProviderChoicesRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "cap:storage", "=provider", "cap:storage=", "cap:storage=/skill:storage", "cap:storage=provider/malformed"} {
		t.Run(value, func(t *testing.T) {
			_, err := parseProviderChoices([]string{value})
			if err == nil || !strings.Contains(err.Error(), "provider choice") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
