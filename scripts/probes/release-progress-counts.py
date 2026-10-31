import importlib.util, json, os, subprocess
from pathlib import Path
root = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('fixture', root / 'scripts/tests/observe-release.py')
f = importlib.util.module_from_spec(spec); spec.loader.exec_module(f)
t = f.ObserverTest(); t.setUp()
try:
    d = f.scenario()
    d['snapshots'] = [f.snapshot(), f.snapshot(), f.snapshot(job_status='queued', step_status='queued'), f.snapshot(job_status='waiting', step_status='queued'), f.snapshot(job_status='waiting', step_status='queued'), f.snapshot(), f.snapshot(status='waiting', job_status='waiting', step_status='queued'), f.snapshot(status='completed', job_status='completed', step_status='completed', result='success')]
    d['pending_by_index'] = {'4': [d['pending'][0]], '5': [d['pending'][0]], '7': [d['pending'][1]]}
    after, calls = t.invoke(d, 'watch', fast=True)
    assert after.returncode == 0 and len(calls) == 11
    assert sum('pending_deployments' in c[-1] for c in calls) == 3
    assert 'completed success' in after.stdout
    observer_bytes = len(after.stdout.encode())
    (t.root / 'scenario').write_text(json.dumps(d)); (t.root / 'calls').unlink(); (t.root / 'index').unlink()
    baseline = []
    def call(args):
        result = subprocess.run(['gh', *args], env=t.env, capture_output=True, text=True, check=True)
        return result.stdout
    baseline.append(call(['run', 'watch', '123', '--repo', 'yersonargotev/packy', '--compact', '--interval', '45', '--exit-status']))
    assert 'run completed success' in baseline[0]
    for _ in d['snapshots']:
        raw = json.loads(call(['run', 'view', '123', '--repo', 'yersonargotev/packy', '--json', 'databaseId,headBranch,headSha,status,jobs']))
        baseline.append(json.dumps({'id': raw['databaseId'], 'tag': raw['headBranch'], 'sha': raw['headSha'], 'status': raw['status'], 'jobs': [{'id': j['databaseId'], 'status': j['status']} for j in raw['jobs']]}) + '\n')
        raw = json.loads(call(['api', 'repos/yersonargotev/packy/actions/runs/123/pending_deployments']))
        baseline.append(json.dumps(raw) + '\n')
    baseline_calls = [json.loads(line) for line in (t.root / 'calls').read_text().splitlines()]
    assert len(baseline_calls) == 17
    approval_calls = 0
    for name, env_id in [('release', '11'), ('homebrew', '22')]:
        result, ac = t.invoke(d, 'check-approval', ['--environment', name, '--environment-id', env_id])
        assert result.returncode == 0 and len(ac) == 3
        approval_calls += len(ac)
    report = {'baseline': {'entry_command_invocations': 17, 'gh_process_invocations': len(baseline_calls), 'output_bytes': sum(len(x.encode()) for x in baseline)}, 'observer': {'entry_command_invocations': 1, 'gh_process_invocations': len(calls), 'output_bytes': observer_bytes}, 'each_path_additional_approval_checks': {'entry_command_invocations': 2, 'gh_process_invocations': approval_calls}, 'diagnostic_gh_invocations': 0, 'timeline_snapshots': len(d['snapshots'])}
    print(json.dumps(report, indent=2))
finally:
    t.tearDown()
