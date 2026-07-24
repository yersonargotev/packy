"""Pure state model for the throwaway Vercel lifecycle prototype."""

from __future__ import annotations

from dataclasses import dataclass, field, replace
from typing import Literal


SKILLS = (
    "vercel-composition-patterns",
    "deploy-to-vercel",
    "vercel-react-best-practices",
    "vercel-react-native-skills",
    "vercel-react-view-transitions",
    "vercel-cli-with-tokens",
    "vercel-optimize",
    "web-design-guidelines",
    "writing-guidelines",
)

Surface = Literal["codex", "opencode", "claude"]
Operation = Literal["activate", "update", "deactivate", "reconcile"]


@dataclass(frozen=True)
class Readiness:
    configured: bool
    authorized: bool
    usable: bool


@dataclass(frozen=True)
class Plan:
    operation: Operation
    surface: Surface
    applicable: bool
    blockers: tuple[str, ...] = ()
    projection_effects: tuple[str, ...] = ()
    preserved: tuple[str, ...] = ()
    pending_human_actions: tuple[str, ...] = ()


@dataclass(frozen=True)
class SurfaceState:
    active: bool = False
    version: str = "—"
    projected: frozenset[str] = frozenset()
    trusted: bool = False
    loaded: frozenset[str] = frozenset()
    collision: str | None = None
    aliases: tuple[tuple[str, str], ...] = ()
    drifted: str | None = None
    optimize_prerequisites: bool = False
    deploy_prerequisites: bool = False

    @property
    def readiness(self) -> Readiness:
        configured = self.active and len(self.projected) == len(SKILLS) and not self.drifted
        authorized = configured and self.trusted
        usable = authorized and len(self.loaded) == len(SKILLS)
        return Readiness(configured, authorized, usable)

    def alias_for(self, public_name: str) -> str | None:
        return dict(self.aliases).get(public_name)


@dataclass(frozen=True)
class Model:
    focus: Surface = "codex"
    surfaces: tuple[tuple[Surface, SurfaceState], ...] = field(
        default_factory=lambda: tuple(
            (surface, SurfaceState()) for surface in ("codex", "opencode", "claude")
        )
    )
    pending_plan: Plan | None = None
    message: str = "Choose a scenario. Every operation is in-memory only."
    runtime_effects_executed: int = 0

    def surface(self, name: Surface | None = None) -> SurfaceState:
        return dict(self.surfaces)[name or self.focus]


def _with_surface(model: Model, value: SurfaceState) -> Model:
    surfaces = tuple(
        (name, value if name == model.focus else state) for name, state in model.surfaces
    )
    return replace(model, surfaces=surfaces)


def _collision_blocker(state: SurfaceState) -> str | None:
    if not state.collision or state.alias_for(state.collision):
        return None
    return (
        f"{state.collision!r} is occupied by an unmanaged resource; "
        "the complete nine-skill surface is blocked before effects"
    )


def preview(model: Model, operation: Operation) -> Model:
    state = model.surface()
    blocker = _collision_blocker(state)
    if operation in ("activate", "update") and blocker:
        return replace(
            model,
            pending_plan=None,
            message=(
                f"{operation.upper()} BLOCKED — {blocker}. "
                "No plan, no intent change, zero effects."
            ),
        )

    if operation == "activate":
        if state.active and state.readiness.configured:
            return replace(
                model,
                pending_plan=None,
                message="ACTIVATE CONVERGED — fresh inspection found no projection change.",
            )
        effects = tuple(f"link inert skill tree: {name}" for name in SKILLS)
        pending = (
            "After projection, satisfy host trust/loading separately.",
            "Invocation tools, authentication, linkage, and mutation permissions remain mode-scoped.",
        )
        plan = Plan(operation, model.focus, True, projection_effects=effects, pending_human_actions=pending)
    elif operation == "update":
        if not state.active:
            return replace(model, pending_plan=None, message="UPDATE INVALID — the pack is inactive.")
        if state.version == "1.1.0" and state.readiness.configured:
            return replace(
                model,
                pending_plan=None,
                message="UPDATE CONVERGED — catalog-current 1.1.0 is already configured.",
            )
        effects = tuple(f"replace inert skill tree: {name}" for name in SKILLS)
        aliases = tuple(f"retain alias {public} → {alias}" for public, alias in state.aliases)
        plan = Plan(operation, model.focus, True, projection_effects=effects, preserved=aliases)
    elif operation == "deactivate":
        if not state.active:
            return replace(
                model,
                pending_plan=None,
                message="DEACTIVATE CONVERGED — the pack is already inactive.",
            )
        removable = tuple(name for name in SKILLS if name != state.drifted)
        effects = tuple(f"remove verified Packy-owned skill tree: {name}" for name in removable)
        preserved = ()
        pending = ()
        if state.drifted:
            preserved = (f"preserve drifted/ambiguous target: {state.drifted}",)
            pending = (f"Inspect and remove {state.drifted!r} manually if desired.",)
        plan = Plan(operation, model.focus, True, projection_effects=effects, preserved=preserved, pending_human_actions=pending)
    else:
        if not state.active:
            return replace(model, pending_plan=None, message="RECONCILE INVALID — the pack is inactive.")
        if not state.drifted:
            return replace(
                model,
                pending_plan=None,
                message="RECONCILE CONVERGED — fresh inspection found all nine trees intact.",
            )
        plan = Plan(
            operation,
            model.focus,
            True,
            projection_effects=(f"restore inert skill tree: {state.drifted}",),
            preserved=("activation intent, aliases, trust, and invocation prerequisites",),
        )

    return replace(
        model,
        pending_plan=plan,
        message=(
            f"{operation.upper()} PREVIEW — exact inert projection plan ready. "
            "No upstream code, authentication, token access, project linkage, Git action, or deployment."
        ),
    )


def approve(model: Model) -> Model:
    plan = model.pending_plan
    if not plan or not plan.applicable:
        return replace(model, message="Nothing applicable is awaiting approval.")
    state = model.surface()
    if plan.operation == "activate":
        state = replace(
            state,
            active=True,
            version="1.0.0",
            projected=frozenset(SKILLS),
            drifted=None,
            loaded=frozenset(),
        )
    elif plan.operation == "update":
        state = replace(
            state,
            version="1.1.0",
            projected=frozenset(SKILLS),
            drifted=None,
            trusted=False,
            loaded=frozenset(),
        )
    elif plan.operation == "deactivate":
        residual = frozenset((state.drifted,)) if state.drifted else frozenset()
        state = replace(
            state,
            active=False,
            version="—",
            projected=residual,
            trusted=False,
            loaded=frozenset(),
        )
    else:
        state = replace(state, projected=frozenset(SKILLS), drifted=None, loaded=frozenset())
    model = _with_surface(model, state)
    return replace(
        model,
        pending_plan=None,
        message=(
            f"{plan.operation.upper()} APPLIED IN MEMORY — verified projection effects only; "
            "runtime authorities executed: 0."
        ),
    )


def dispatch(model: Model, command: str) -> Model:
    parts = command.strip().lower().split()
    if not parts:
        return model
    verb = parts[0]
    if verb == "surface" and len(parts) == 2 and parts[1] in ("codex", "opencode", "claude"):
        return replace(model, focus=parts[1], pending_plan=None, message=f"Focused {parts[1]}.")
    if verb == "collision":
        state = model.surface()
        collision = None if state.collision else "web-design-guidelines"
        state = replace(state, collision=collision)
        model = _with_surface(model, state)
        return replace(model, pending_plan=None, message=f"Observed collision: {collision or 'none'}.")
    if verb == "alias":
        state = model.surface()
        if not state.collision:
            return replace(model, message="No collision needs an alias.")
        aliases = dict(state.aliases)
        aliases[state.collision] = f"vercel-pack-{state.collision}"
        state = replace(state, aliases=tuple(sorted(aliases.items())))
        model = _with_surface(model, state)
        return replace(model, pending_plan=None, message="Explicit surface-local alias selected; preview again.")
    if verb in ("activate", "update", "deactivate", "reconcile"):
        return preview(model, verb)
    if verb == "approve":
        return approve(model)
    if verb == "trust":
        state = replace(model.surface(), trusted=True)
        model = _with_surface(model, state)
        return replace(model, message="Host trust observed; Packy performed no trust action.")
    if verb == "load":
        state = replace(model.surface(), loaded=frozenset(SKILLS))
        model = _with_surface(model, state)
        return replace(model, message="Host discovery/load observed for all nine skills.")
    if verb == "drift":
        state = model.surface()
        drifted = "vercel-react-best-practices"
        state = replace(state, projected=state.projected - {drifted}, loaded=frozenset(), drifted=drifted)
        model = _with_surface(model, state)
        return replace(model, pending_plan=None, message=f"Simulated external drift of {drifted}.")
    if verb == "enable" and len(parts) == 2 and parts[1] in ("optimize", "deploy"):
        state = model.surface()
        if parts[1] == "optimize":
            state = replace(state, optimize_prerequisites=True)
        else:
            state = replace(state, deploy_prerequisites=True)
        model = _with_surface(model, state)
        return replace(model, message=f"Simulated verified invocation prerequisites for {parts[1]}.")
    if verb == "mode" and len(parts) == 2 and parts[1] in ("optimize", "deploy"):
        state = model.surface()
        if parts[1] == "optimize":
            availability = "available with verified sequential fallback" if state.optimize_prerequisites else "unavailable"
            message = (
                f"MODE vercel-optimize — {availability}. Tools: Node.js 20+, Vercel CLI 53+. "
                "Needs auth, linkage, metrics/entitlements as requested. Authorities: process, network, "
                "project reads/writes, service data. Fallback: sequential investigation when subagents "
                "are unavailable. Missing indispensable prerequisites stop this invocation before effects."
            )
        else:
            availability = "available" if state.deploy_prerequisites else "unverified"
            message = (
                f"MODE deploy-to-vercel — {availability}. Needs Vercel auth, project linkage, network, "
                "and explicit mutation/deployment permission. Authorities: process, Git, upload, preview "
                "or production deployment. Fallback: only included declared routes. Tokens: presence only; "
                "values and token-bearing commands are never displayed. Missing permission stops before effects."
            )
        return replace(model, message=message)
    if verb == "reset":
        return Model()
    return replace(model, message=f"Unknown command: {command!r}")
