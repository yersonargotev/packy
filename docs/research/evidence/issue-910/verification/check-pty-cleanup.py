import os, subprocess, sys, tempfile, time, json
from pathlib import Path
root=Path.cwd(); helper=root/'.agents/skills/verify-packy/scripts/pty-drive.py'
results=[]
for status in (0,7):
 with tempfile.TemporaryDirectory(prefix='packy-910-pty-control-') as temp:
  scratch=Path(temp); (scratch/'home').mkdir(); (scratch/'xdg').mkdir()
  env={'PATH':'/usr/bin:/bin','HOME':str(scratch/'home'),'XDG_CONFIG_HOME':str(scratch/'xdg'),'TERM':'xterm-256color','PYTHONDONTWRITEBYTECODE':'1'}
  fake=scratch/'packy'
  fake.write_text('#!'+sys.executable+'\nimport subprocess,sys,os\nfrom pathlib import Path\nchild=subprocess.Popen([sys.executable,"-c","import time; time.sleep(90)"],env=os.environ.copy())\nPath("child.pid").write_text(str(child.pid))\nprint("READY",flush=True)\ninput()\nsys.exit('+str(status)+')\n'); fake.chmod(0o700)
  transcript=scratch/'trace.raw'
  result=subprocess.run([sys.executable,'-B',str(helper),str(fake),str(transcript),'--step','READY','ENTER'],env=env,cwd=scratch,capture_output=True,text=True,timeout=45)
  pid=int((scratch/'child.pid').read_text())
  time.sleep(.1)
  state=subprocess.run(['/bin/ps','-o','stat=','-p',str(pid)],env=env,capture_output=True,text=True).stdout.strip()
  assert not state or state.startswith('Z'), (pid,state)
  assert (result.returncode==0)==(status==0),(status,result.stdout,result.stderr)
  assert b'READY' in transcript.read_bytes()
  retained=Path('/tmp/packy-910-run')/f'pty-control-{status}.raw'; retained.write_bytes(transcript.read_bytes())
  results.append({'leader_exit':status,'helper_exit':result.returncode,'descendant_state':state or 'absent','transcript_retained':str(retained),'stdout':result.stdout,'stderr':result.stderr})
print(json.dumps(results,indent=2))
