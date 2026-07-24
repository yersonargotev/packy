#!/usr/bin/env python3
"""THROWAWAY interactive shell for the Vercel lifecycle behavior model."""

from __future__ import annotations

from model import Model, SKILLS, dispatch


BOLD = "\033[1m"
DIM = "\033[2m"
RESET = "\033[0m"


def yes(value: bool) -> str:
    return "yes" if value else "no"


def render(model: Model) -> None:
    print("\033[2J\033[H", end="")
    print(f"{BOLD}Vercel capability-pack behavior prototype{RESET}")
    print(f"{DIM}All state is in memory; runtime authorities executed: {model.runtime_effects_executed}{RESET}\n")
    print(f"{BOLD}SURFACE OVERVIEW{RESET}")
    print("FOCUS  SURFACE   INTENT    VERSION  PROJECTED  CONFIGURED  AUTHORIZED  USABLE")
    for name, state in model.surfaces:
        ready = state.readiness
        print(
            f"{'>' if name == model.focus else ' ':5}  {name:9} "
            f"{'active' if state.active else 'inactive':9} {state.version:7} "
            f"{len(state.projected):>2}/{len(SKILLS):<6} {yes(ready.configured):11} "
            f"{yes(ready.authorized):11} {yes(ready.usable)}"
        )

    state = model.surface()
    print(f"\n{BOLD}FOCUSED SURFACE: {model.focus}{RESET}")
    print(f"collision: {state.collision or 'none'}")
    print(f"aliases: {dict(state.aliases) or 'none'}")
    print(f"drift: {state.drifted or 'none'}")
    print(
        "invocation modes: "
        f"optimize={'verified' if state.optimize_prerequisites else 'unavailable'}; "
        f"deploy={'verified' if state.deploy_prerequisites else 'unverified'}"
    )

    if model.pending_plan:
        plan = model.pending_plan
        print(f"\n{BOLD}PENDING {plan.operation.upper()} PLAN{RESET}")
        for blocker in plan.blockers:
            print(f"  BLOCK: {blocker}")
        for effect in plan.projection_effects:
            print(f"  {effect}")
        for item in plan.preserved:
            print(f"  preserve: {item}")
        for item in plan.pending_human_actions:
            print(f"  human: {item}")
        print("  runtime/upstream effects: none")

    print(f"\n{BOLD}RESULT{RESET}\n{model.message}")
    print(f"\n{BOLD}COMMANDS{RESET}")
    print("surface codex|opencode|claude   collision   alias")
    print("activate   update   deactivate   reconcile   approve")
    print("trust   load   drift   mode optimize|deploy")
    print("enable optimize|deploy   reset   quit")


def main() -> None:
    model = Model()
    while True:
        render(model)
        try:
            command = input("\n> ")
        except (EOFError, KeyboardInterrupt):
            print()
            return
        if command.strip().lower() in ("quit", "q"):
            return
        model = dispatch(model, command)


if __name__ == "__main__":
    main()
