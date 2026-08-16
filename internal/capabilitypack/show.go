package capabilitypack

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const packSourceIdentityLimitation = "Packy records the trusted pack ID, version, and manifest schema, but no upstream source provenance."

// PackSourceIdentity is the stable source identity Packy can derive from its
// trusted domain facts. Upstream repository provenance is not part of Pack.
type PackSourceIdentity struct {
	PackID        string `json:"pack_id"`
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
	Limitation    string `json:"limitation"`
}

// ShowIntent reports the durable surface-local intent without observing a
// host. Present distinguishes no record from an inactive record.
type ShowIntent struct {
	Present  bool
	Active   bool
	Version  string
	Revision int
	Aliases  []SurfaceAlias
}

// ShowSurfaceReport contains the deterministic portable contract and durable
// intent facts for one supported surface.
type ShowSurfaceReport struct {
	Surface  Surface
	Contract LifecycleContract
	Intent   ShowIntent
}

// ResourceInventoryRole describes how a resource contributes to a Pack,
// independently of any lifecycle selection or host projection.
type ResourceInventoryRole string

const (
	ResourceInventoryRoleOperational ResourceInventoryRole = "operational"
	ResourceInventoryRoleSupporting  ResourceInventoryRole = "supporting"
	ResourceInventoryRoleNotice      ResourceInventoryRole = "notice"
)

// DescriptiveResource is the domain-owned discovery contract for one Pack
// resource. Dependencies and notices contain direct manifest relationships.
type DescriptiveResource struct {
	Resource     ResourceIdentity      `json:"resource"`
	Description  string                `json:"description"`
	Role         ResourceInventoryRole `json:"role"`
	Dependencies []ResourceIdentity    `json:"dependencies"`
	Notices      []ResourceIdentity    `json:"notices"`
}

// ShowReport is the detached domain result used by pack show renderers.
type ShowReport struct {
	Detail                CatalogDetail
	SourceIdentity        PackSourceIdentity
	ResourceCounts        ResourceCounts
	ResourceInventory     []DescriptiveResource
	ResourceGraph         ResourceGraph
	LifecycleAvailability ShowLifecycleAvailability
	Surfaces              []ShowSurfaceReport
}

// ShowDecisionSummary is the owner-provided human decision that renderers place
// before the complete portable contract.
type ShowDecisionSummary struct {
	WhatWillChange string
	Risks          []string
	NextCommand    string
}

// DecisionSummary derives inspection guidance from the same catalog, intent,
// and selection-validity facts that own lifecycle policy.
func (report ShowReport) DecisionSummary() ShowDecisionSummary {
	pack := report.Detail.Pack
	risks := make([]string, 0)
	requiresTools := append([]string(nil), pack.Requires.Tools...)
	sort.Strings(requiresTools)
	if len(requiresTools) > 0 {
		risks = append(risks, "requires global tools "+strings.Join(requiresTools, ", "))
	}
	for _, surface := range report.Surfaces {
		if surface.Contract.SelectionValidity.All.Available {
			continue
		}
		if len(surface.Contract.SelectionValidity.All.Reasons) == 0 {
			risks = append(risks, fmt.Sprintf("%s all-resource selection is unavailable", surface.Surface))
			continue
		}
		for _, reason := range surface.Contract.SelectionValidity.All.Reasons {
			risks = append(risks, fmt.Sprintf(
				"%s all-resource selection is unavailable: %s; remediation: %s",
				surface.Surface, reason.Detail, reason.Remediation,
			))
		}
	}

	changes := make([]string, 0, len(report.Surfaces))
	for _, surface := range report.Surfaces {
		intent := surface.Intent
		switch {
		case intent.Present && intent.Active && intent.Version != pack.Version && report.LifecycleAvailability.CatalogUpdateAvailable:
			changes = append(changes, fmt.Sprintf("update %s from %s to %s", surface.Surface, intent.Version, pack.Version))
		case intent.Present && intent.Active:
			changes = append(changes, fmt.Sprintf("keep %s at %s", surface.Surface, intent.Version))
		case intent.Present && report.LifecycleAvailability.FreshActivationAvailable && surface.Contract.SelectionValidity.All.Available:
			changes = append(changes, fmt.Sprintf("activate inactive %s intent at %s", surface.Surface, intent.Version))
		case intent.Present:
			changes = append(changes, fmt.Sprintf("no activation is available for inactive %s intent", surface.Surface))
		}
	}
	if len(changes) == 0 {
		available := make([]Surface, 0, len(report.Surfaces))
		for _, surface := range report.Surfaces {
			if report.LifecycleAvailability.FreshActivationAvailable && surface.Contract.SelectionValidity.All.Available {
				available = append(available, surface.Surface)
			}
		}
		if len(available) == 0 {
			changes = append(changes, fmt.Sprintf(
				"no compatible surface is available for %d catalog resources",
				showResourceTotal(report.ResourceCounts),
			))
		} else {
			changes = append(changes, fmt.Sprintf(
				"activation would manage %d catalog resources across %s",
				showResourceTotal(report.ResourceCounts), joinShowSurfaces(available),
			))
		}
	}

	nextCommand := "packy list"
	for _, surface := range report.Surfaces {
		if surface.Intent.Present && surface.Intent.Active &&
			surface.Intent.Version != pack.Version && report.LifecycleAvailability.CatalogUpdateAvailable {
			nextCommand = fmt.Sprintf("packy update %s --surface %s --dry-run", pack.ID, surface.Surface)
			return ShowDecisionSummary{WhatWillChange: strings.Join(changes, "; "), Risks: risks, NextCommand: nextCommand}
		}
	}
	for _, surface := range report.Surfaces {
		if surface.Intent.Present && surface.Intent.Active {
			nextCommand = fmt.Sprintf("packy status %s --surface %s", pack.ID, surface.Surface)
			return ShowDecisionSummary{WhatWillChange: strings.Join(changes, "; "), Risks: risks, NextCommand: nextCommand}
		}
	}
	for _, surface := range report.Surfaces {
		if report.LifecycleAvailability.FreshActivationAvailable && surface.Contract.SelectionValidity.All.Available {
			nextCommand = fmt.Sprintf("packy activate %s --surface %s --dry-run", pack.ID, surface.Surface)
			break
		}
	}
	return ShowDecisionSummary{WhatWillChange: strings.Join(changes, "; "), Risks: risks, NextCommand: nextCommand}
}

func showResourceTotal(counts ResourceCounts) int {
	return counts.Skills + counts.Instructions + counts.MCPServers + counts.Lifecycles +
		counts.Agents + counts.Commands + counts.Assets + counts.Notices
}

func joinShowSurfaces(surfaces []Surface) string {
	values := make([]string, len(surfaces))
	for i, surface := range surfaces {
		values[i] = string(surface)
	}
	return strings.Join(values, ", ")
}

// ShowLifecycleAvailability makes withdrawal remediation explicit without
// conflating catalog selection with lifecycle access for existing intents.
type ShowLifecycleAvailability struct {
	FreshActivationAvailable bool
	CatalogUpdateAvailable   bool
	LifecycleVerbsAvailable  bool
	AutomaticDowngrade       bool
}

// Show returns catalog metadata, portable per-surface contracts, and durable
// surface-local intent facts. It performs no host inspection or mutation.
func (f Facade) Show(ctx context.Context, id string) (ShowReport, error) {
	return withBundleObservation(ctx, f, func(locked Facade) (ShowReport, error) {
		return locked.show(ctx, id)
	})
}

func (f Facade) show(ctx context.Context, id string) (ShowReport, error) {
	detail, err := f.catalog.ShowDetail(ctx, id)
	if err != nil {
		return ShowReport{}, err
	}
	if f.activation == nil || f.activation.store == nil {
		return ShowReport{}, fmt.Errorf("surface intent observation is not configured")
	}

	pack := detail.Pack
	report := ShowReport{
		Detail: detail,
		SourceIdentity: PackSourceIdentity{
			PackID:        pack.ID,
			Version:       pack.Version,
			SchemaVersion: pack.manifestVersion,
			Limitation:    packSourceIdentityLimitation,
		},
		ResourceCounts:    pack.ResourceCounts(),
		ResourceInventory: detail.ResourceInventory,
		ResourceGraph:     ResourceGraphFor(pack, ResourceSelection{Mode: SelectionAll, Roots: []ResourceIdentity{}}, true),
		LifecycleAvailability: ShowLifecycleAvailability{
			FreshActivationAvailable: true,
			CatalogUpdateAvailable:   true,
			LifecycleVerbsAvailable:  true,
			AutomaticDowngrade:       false,
		},
		Surfaces: make([]ShowSurfaceReport, 0, len(pack.Surfaces)),
	}
	surfaces := append([]Surface(nil), pack.Surfaces...)
	sort.Slice(surfaces, func(i, j int) bool { return surfaces[i] < surfaces[j] })
	for _, surface := range surfaces {
		state, err := f.activation.store.LoadSnapshot(ctx, surface)
		if err != nil {
			return ShowReport{}, fmt.Errorf("load %s surface intent: %w", surface, err)
		}
		intent, present := intentForPack(state, pack.ID, surface)
		aliases := []SurfaceAlias{}
		if present {
			aliases = canonicalShowAliases(intent.Aliases)
		}
		report.Surfaces = append(report.Surfaces, ShowSurfaceReport{
			Surface:  surface,
			Contract: LifecycleContractFor(pack, surface, aliases),
			Intent: ShowIntent{
				Present:  present,
				Active:   present && intent.Active,
				Version:  intent.Version,
				Revision: intent.Revision,
				Aliases:  cloneAliases(aliases),
			},
		})
	}
	return report, nil
}

func descriptiveResourceInventory(pack Pack) []DescriptiveResource {
	resources := make([]DescriptiveResource, 0, len(pack.Resources))
	for _, resource := range pack.Resources {
		role := ResourceInventoryRoleOperational
		switch resource.Kind {
		case "asset":
			role = ResourceInventoryRoleSupporting
		case "notice":
			role = ResourceInventoryRoleNotice
		}
		dependencies := resourceIdentities(resource.Requires)
		notices := resourceIdentities(resource.Notices)
		sort.Slice(dependencies, func(i, j int) bool { return dependencies[i].String() < dependencies[j].String() })
		sort.Slice(notices, func(i, j int) bool { return notices[i].String() < notices[j].String() })
		resources = append(resources, DescriptiveResource{
			Resource:     ResourceIdentity{Kind: resource.Kind, ID: resource.ID},
			Description:  resource.Description,
			Role:         role,
			Dependencies: dependencies,
			Notices:      notices,
		})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Resource.String() < resources[j].Resource.String() })
	return resources
}

func canonicalShowAliases(aliases []SurfaceAlias) []SurfaceAlias {
	result := cloneAliases(aliases)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind != result[j].Kind {
			return result[i].Kind < result[j].Kind
		}
		if result[i].ID != result[j].ID {
			return result[i].ID < result[j].ID
		}
		return result[i].Name < result[j].Name
	})
	return result
}
