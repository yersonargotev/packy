package catalogstore

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yersonargotev/packy/internal/capabilitypack"
)

// DefaultWorkflow is the publication workflow an attested registration names
// when the operator gives none.
const DefaultWorkflow = ".github/workflows/publish.yml"

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
	return Registration{Repository: Repository{owner: "yersonargotev", name: "packy-catalog"}, Trust: Attested{Workflow: DefaultWorkflow}}
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

// ParseTrustPolicy returns the trust policy that name and workflow describe.
// Only an attested policy names a publication workflow, and an attested policy
// without one names DefaultWorkflow.
func ParseTrustPolicy(name, workflow string) (TrustPolicy, error) {
	switch name {
	case "attested":
		if workflow == "" {
			workflow = DefaultWorkflow
		}
		return Attested{Workflow: workflow}, nil
	case "access-controlled":
		if workflow != "" {
			return nil, errors.New("an access-controlled catalog names no publication workflow")
		}
		return AccessControlled{}, nil
	}
	return nil, fmt.Errorf("trust policy must be attested or access-controlled, not %q", name)
}

// TrustPolicyName returns the name and publication workflow of a trust
// policy, as ParseTrustPolicy reads them.
func TrustPolicyName(trust TrustPolicy) (name, workflow string) {
	switch trust := trust.(type) {
	case Attested:
		return "attested", trust.Workflow
	case AccessControlled:
		return "access-controlled", ""
	}
	return "", ""
}

func encodeRegistration(registration Registration) registrationDocument {
	document := registrationDocument{SchemaVersion: 1, Repository: registration.Repository.String()}
	document.Trust, document.Workflow = TrustPolicyName(registration.Trust)
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
	if document.Trust == "attested" && document.Workflow == "" {
		return Registration{}, fmt.Errorf("attested catalog %s registration names no publication workflow", repository)
	}
	trust, err := ParseTrustPolicy(document.Trust, document.Workflow)
	if err != nil {
		return Registration{}, fmt.Errorf("catalog %s: %w", repository, err)
	}
	registration := Registration{Repository: repository, Trust: trust}
	return registration, registration.validate()
}
