package managedpackpromotion_test

import (
	"testing"

	"github.com/yersonargotev/packy/internal/managedpackpromotion"
)

func TestParseCoordinateAcceptsOnePackAndExactVersion(t *testing.T) {
	coordinate, err := managedpackpromotion.ParseCoordinate("issue-delivery@1.2.3")
	if err != nil {
		t.Fatalf("ParseCoordinate returned an error: %v", err)
	}
	if coordinate.PackID != "issue-delivery" || coordinate.Version != "1.2.3" {
		t.Fatalf("ParseCoordinate = %#v, want issue-delivery@1.2.3", coordinate)
	}
	if coordinate.String() != "issue-delivery@1.2.3" {
		t.Fatalf("Coordinate.String() = %q", coordinate.String())
	}
}

func TestParseCoordinateRejectsAnythingOtherThanOneStrictCoordinate(t *testing.T) {
	for _, value := range []string{
		"", "addy", "addy@", "@1.2.3", "Addy@1.2.3", "addy_thing@1.2.3",
		"addy@v1.2.3", "addy@1.2", "addy@1.2.3@other",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := managedpackpromotion.ParseCoordinate(value); err == nil {
				t.Fatalf("ParseCoordinate(%q) succeeded", value)
			}
		})
	}
}
