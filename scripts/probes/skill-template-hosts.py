#!/usr/bin/env python3
"""Opt-in discovery checks against skill-template-catalog.py owned workspaces.

Codex lists skills without starting a thread. Claude sends to an explicit
localhost recorder; no credentials or model inference.
"""
import argparse
import http.server
import json
import os
from pathlib import Path
import selectors
import shutil
import signal
import subprocess
import threading
import time


def environment(workspace):
    env = {"PATH": "/usr/bin:/bin:/usr/sbin:/sbin", "LANG": "en_US.UTF-8", "TERM": "dumb"}
    for name, directory in (("HOME", "home"), ("XDG_CONFIG_HOME", "config"),
                            ("XDG_DATA_HOME", "host-data"), ("XDG_STATE_HOME", "host-state"),
                            ("XDG_CACHE_HOME", "host-cache"), ("TMPDIR", "host-tmp")):
        path = workspace / directory
        path.mkdir(exist_ok=True)
        env[name] = str(path)
    env["CODEX_HOME"] = str(workspace / "home/.codex")
    env["CLAUDE_CONFIG_DIR"] = str(workspace / "home/.claude")
    Path(env["CODEX_HOME"]).mkdir(exist_ok=True)
    Path(env["CLAUDE_CONFIG_DIR"]).mkdir(exist_ok=True)
    return env


def codex_list(binary, workspace, env):
    process = subprocess.Popen([binary, "app-server", "--stdio"], cwd=workspace / "project", env=env,
                               stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL,
                               start_new_session=True, text=True)
    def send(value):
        process.stdin.write(json.dumps(value) + "\n")
        process.stdin.flush()
    def response(wanted):
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ)
            deadline = time.monotonic() + 30
            while time.monotonic() < deadline:
                if not selector.select(min(1, deadline - time.monotonic())):
                    continue
                line = process.stdout.readline()
                if not line:
                    raise RuntimeError("Codex app-server exited before its response")
                value = json.loads(line)
                if value.get("id") == wanted:
                    if "error" in value:
                        raise RuntimeError(str(value["error"]))
                    return value["result"]
        raise RuntimeError("Codex app-server deadline expired")
    try:
        send({"method": "initialize", "id": 0, "params": {"clientInfo": {"name": "packy_acceptance", "version": "1.0.0"}}})
        response(0)
        send({"method": "initialized", "params": {}})
        send({"method": "skills/list", "id": 1, "params": {"cwds": [str(workspace / "project")], "forceReload": True}})
        result = response(1)
        skills = [skill for item in result["data"] for skill in item["skills"]]
        names = {item["name"] for item in skills}
        expected = {"tdd", "teach", "pstack-tdd", "pstack-teach"}
        if not expected <= names:
            raise RuntimeError("Codex omitted names: " + str(expected - names))
        errors = [error for item in result["data"] for error in item.get("errors", [])]
        if errors:
            raise RuntimeError("Codex skill errors: " + str(errors))
        return {"native_names": sorted(expected), "errors": errors, "model_inference": False}
    finally:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait()
        process.stdin.close()
        process.stdout.close()


def run_host(command, workspace, env):
    process = subprocess.Popen(command, cwd=workspace / "project", env=env,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                               start_new_session=True, text=True)
    try:
        stdout, stderr = process.communicate(timeout=55)
        return subprocess.CompletedProcess(command, process.returncode, stdout, stderr)
    finally:
        try:
            os.killpg(process.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        process.wait()
        process.stdout.close()
        process.stderr.close()


def contains_body(value, body):
    if isinstance(value, str):
        return body in value.replace("\r\n", "\n")
    if isinstance(value, dict):
        return any(contains_body(item, body) for item in value.values())
    if isinstance(value, list):
        return any(contains_body(item, body) for item in value)
    return False


def claude_load(binary, workspace, env, skill_name, expected_body):
    requests = []
    class Provider(http.server.BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            requests.append(body)
            self.send_response(400)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"type":"error","error":{"type":"invalid_request_error","message":"packy pre-inference probe"}}')
    server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Provider)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    url = "http://127.0.0.1:" + str(server.server_port)
    env = dict(env)
    env.update(ANTHROPIC_API_KEY="fake-local-only", ANTHROPIC_BASE_URL=url,
               CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC="1")
    command = [binary, "-p", "/" + skill_name + " Show the supplied workflow only. Do not run it.",
               "--output-format", "stream-json", "--verbose", "--no-session-persistence", "--tools", "",
               "--strict-mcp-config", "--mcp-config", '{"mcpServers":{}}']
    try:
        result = run_host(command, workspace, env)
        if not contains_body(requests, expected_body):
            raise RuntimeError("Claude did not load " + skill_name + ": " + result.stderr[-1000:])
        return {"name": skill_name, "body_loaded": True, "requests": len(requests),
                "exit_status": result.returncode, "model_inference": False}
    finally:
        server.shutdown()
        server.server_close()
        thread.join(timeout=2)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--catalog-evidence", type=Path, required=True)
    parser.add_argument("--agents-client", choices=("codex",), help="explicit concrete client for Agents discovery")
    args = parser.parse_args()
    evidence = json.loads(args.catalog_evidence.read_text())
    if evidence.get("result") != "passed":
        raise RuntimeError("Catalog acceptance must pass before real-host probes")
    if any(case["surface"] == "agents" for case in evidence["outcomes"]) and not args.agents_client:
        parser.error("Agents discovery requires --agents-client codex; the surface does not select a runtime")
    results = []
    for case in evidence["outcomes"]:
        surface, scope = case["surface"], case["scope"]
        workspace = Path(case["workspace"])
        if surface not in ("agents", "claude"):
            raise RuntimeError("unsupported probe surface: " + surface)
        client = args.agents_client if surface == "agents" else "claude"
        binary = shutil.which(client)
        if not binary:
            raise RuntimeError("host executable is unavailable: " + client)
        env = environment(workspace)
        version = subprocess.run([binary, "--version"], env=env, capture_output=True, text=True, check=True, timeout=15).stdout.strip()
        if client == "codex":
            result = codex_list(binary, workspace, env)
        else:
            loads = []
            for skill_name in ("tdd", "teach", "pstack-tdd", "pstack-teach"):
                native = Path(case["skills"]) / skill_name / "SKILL.md"
                content = native.read_text().replace("\r\n", "\n")
                body = content.split("\n---\n", 1)[1].strip()
                loads.append(claude_load(binary, workspace, env, skill_name, body))
            result = {"loads": loads}
        results.append({"surface": surface, "client": client, "scope": scope, "version": version, **result})
    summary = {"report": "skill-template-host-loading", "result": "passed", "outcomes": results}
    (args.catalog_evidence.parent / "host-evidence.json").write_text(json.dumps(summary, indent=2) + "\n")
    print(json.dumps(summary, indent=2))


if __name__ == "__main__":
    main()
