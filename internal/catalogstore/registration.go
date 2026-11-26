package catalogstore

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

const officialPublicationWorkflow = ".github/workflows/publish.yml"

var (
	workflowPattern = regexp.MustCompile(`^\.github/workflows/[A-Za-z0-9_.-]+\.ya?ml$`)
)

// Repository is a GitHub catalog repository in canonical lowercase
// owner/name form. GitHub resolves repository names case-insensitively.
type Repository struct{ owner, name string }

// ParseRepository lowercases an ASCII GitHub owner/name to its canonical form
// and rejects any other value.
func ParseRepository(value string) (Repository, error) {
	canonical, err := capabilitypack.CanonicalCatalogRepository(value)
	if err != nil {
		return Repository{}, err
	}
	owner, name, _ := strings.Cut(canonical, "/")
	return Repository{owner: owner, name: name}, nil
}

func (r Repository) String() string { return r.owner + "/" + r.name }

// TrustPolicy is the publisher evidence a catalog registration requires. It is
// Attested or AccessControlled.
type TrustPolicy interface{ trustPolicy() }

// Attested requires a verified GitHub build-provenance attestation of the
// archive, signed by Workflow of the catalog repository on refs/heads/main.
type Attested struct{ Workflow string }

// AccessControlled accepts GitHub access control of a private repository in
// place of an attestation.
type AccessControlled struct{}

func (Attested) trustPolicy()         {}
func (AccessControlled) trustPolicy() {}

// Registration binds a catalog repository to its fixed trust policy.
type Registration struct {
	Repository Repository
	Trust      TrustPolicy
}

// Official returns the built-in registration of the official catalog.
func Official() Registration {
	return Registration{Repository: Repository{owner: "yersonargotev", name: "packy-catalog"}, Trust: Attested{Workflow: officialPublicationWorkflow}}
}

func (r Registration) validate() error {
	if r.Repository == (Repository{}) {
		return errors.New("catalog registration requires a repository")
	}
	switch trust := r.Trust.(type) {
	case Attested:
		if !workflowPattern.MatchString(trust.Workflow) {
			return fmt.Errorf("catalog %s publication workflow %q is not a file in .github/workflows", r.Repository, trust.Workflow)
		}
	case AccessControlled:
	default:
		return fmt.Errorf("catalog %s requires an attested or access-controlled trust policy", r.Repository)
	}
	return nil
}

type registrationDocument struct {
	SchemaVersion int    `json:"schema_version"`
	Repository    string `json:"repository"`
	Trust         string `json:"trust"`
	Workflow      string `json:"workflow,omitempty"`
}

func encodeRegistration(registration Registration) registrationDocument {
	document := registrationDocument{SchemaVersion: 1, Repository: registration.Repository.String(), Trust: "access-controlled"}
	if attested, ok := registration.Trust.(Attested); ok {
		document.Trust, document.Workflow = "attested", attested.Workflow
	}
	return document
}

func decodeRegistration(document registrationDocument) (Registration, error) {
	if document.SchemaVersion != 1 {
		return Registration{}, errors.New("catalog registration schema_version must be 1")
	}
	repository, err := ParseRepository(document.Repository)
	if err != nil {
		return Registration{}, err
	}
	registration := Registration{Repository: repository}
	switch document.Trust {
	case "attested":
		registration.Trust = Attested{Workflow: document.Workflow}
	case "access-controlled":
		if document.Workflow != "" {
			return Registration{}, fmt.Errorf("access-controlled catalog %s names a publication workflow", repository)
		}
		registration.Trust = AccessControlled{}
	default:
		return Registration{}, fmt.Errorf("catalog %s has unknown trust policy %q", repository, document.Trust)
	}
	return registration, registration.validate()
}
