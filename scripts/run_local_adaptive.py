"""Supervise local Go HTTP + independent Python ML replicas. Ctrl-C stops all.

Run this with the Python environment containing ml-service/requirements.txt.
No dependency installation, model download or termination of existing services.
"""
import argparse
import json
import os
from pathlib import Path
import secrets
import signal
import socket
import subprocess
import sys
import time
import urllib.request


def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('--replicas',type=int,default=6)
    p.add_argument('--quality-workers',type=int,default=1)
    p.add_argument('--spacy-workers',type=int,default=2)
    p.add_argument('--http-port',type=int,default=8080)
    p.add_argument('--base-port',type=int,default=50051)
    p.add_argument('--metrics-base',type=int,default=9091)
    p.add_argument('--backend',choices=['adaptive','quality','spacy_sm'],default='adaptive')
    p.add_argument('--runtime-dir',default='.runtime/adaptive')
    p.add_argument('--routing-budget-ms',type=float,default=100)
    p.add_argument('--binary',default=None)
    p.add_argument('--auth-mode',choices=['api_key','verify'],default='api_key',
                   help='verify disables authentication for testing; inference remains real')
    args=p.parse_args()
    if min(args.replicas,args.quality_workers,args.spacy_workers)<=0: p.error('worker counts must be positive')
    root=Path(__file__).resolve().parents[1]
    runtime=(root/args.runtime_dir).resolve();runtime.mkdir(parents=True,exist_ok=True)
    ports=[args.http_port]+[args.base_port+i for i in range(args.replicas)]+[args.metrics_base+i for i in range(args.replicas)]
    if len(set(ports))!=len(ports) or not all(0<port<65536 for port in ports):p.error('invalid/overlapping ports')
    for port in ports:
        with socket.socket() as sock:
            if sock.connect_ex(('127.0.0.1',port))==0:raise SystemExit(f'Port {port} already in use; existing service left untouched')
    binary=Path(args.binary).resolve() if args.binary else runtime/'alpha-proxy'
    if not args.binary:
        subprocess.run(['go','build','-o',str(binary),'./cmd/server'],cwd=root,check=True)
    auth=runtime/'systems.json'
    if not auth.exists():
        fd=os.open(auth,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
        with os.fdopen(fd,'w') as out:json.dump([dict(id='local',enabled=True,api_key=secrets.token_urlsafe(32))],out)
    children=[]; logs=[]; stopping=False
    def stop(signum=None,frame=None):
        nonlocal stopping
        stopping=True
    signal.signal(signal.SIGTERM,stop);signal.signal(signal.SIGINT,stop)
    def spawn(command,cwd,env,name):
        log=(runtime/f'{name}.log').open('ab',buffering=0);logs.append(log)
        child=subprocess.Popen(command,cwd=cwd,env=env,stdout=log,stderr=subprocess.STDOUT)
        children.append(child)
        return child
    try:
        targets=[]
        for i in range(args.replicas):
            env=os.environ.copy()
            env.update(ML_BACKEND=args.backend,ML_QUALITY_WORKERS=str(args.quality_workers),ML_SPACY_WORKERS=str(args.spacy_workers),
                ML_QUALITY_QUEUE='2' if args.backend=='adaptive' else '128',ML_SPACY_QUEUE='128',
                ML_GRPC_HOST='127.0.0.1',ML_GRPC_PORT=str(args.base_port+i),ML_METRICS_HOST='127.0.0.1',
                ML_METRICS_PORT=str(args.metrics_base+i),ML_GRPC_MAX_WORKERS='16',ML_GRPC_MAX_CONCURRENT_RPCS='32',
                ML_ROUTING_BUDGET_MS=str(args.routing_budget_ms),ML_MAX_BATCH_CHUNKS='32',
                ML_INTRA_OP_THREADS='1',ML_INTER_OP_THREADS='1',ML_TENSOR_BATCH_SIZE='1',
                OMP_NUM_THREADS='1',OPENBLAS_NUM_THREADS='1',VECLIB_MAXIMUM_THREADS='1',MKL_NUM_THREADS='1',
                NUMEXPR_NUM_THREADS='1',TOKENIZERS_PARALLELISM='false',PYTHONUNBUFFERED='1')
            spawn([sys.executable,'-m','ml_service'],root/'ml-service',env,f'ml-{i}')
            targets.append(f'127.0.0.1:{args.base_port+i}')
        import grpc
        for target in targets:
            with grpc.insecure_channel(target) as channel:grpc.channel_ready_future(channel).result(timeout=30)
        env=os.environ.copy()
        env.update(ALPHA_PROXY_ADDR=f'127.0.0.1:{args.http_port}',ALPHA_PROXY_RUN_MODE='verify' if args.auth_mode=='verify' else 'final',
            ALPHA_PROXY_AUTH_MODE=args.auth_mode,ALPHA_PROXY_PROCESSOR_MODE='real',ALPHA_PROXY_SYSTEMS_FILE=str(auth),
            ALPHA_PROXY_ML_ADDR=','.join(targets),ALPHA_PROXY_ML_RPC_WORKERS=str(args.replicas*4),
            ALPHA_PROXY_ML_BATCH_ITEMS='32',ALPHA_PROXY_ML_BATCH_WAIT='1ms',ALPHA_PROXY_ML_RPC_TIMEOUT='2s',
            ALPHA_PROXY_GLOBAL_RATE_LIMIT_RPS='0',ALPHA_PROXY_GLOBAL_RATE_LIMIT_BURST='2000',
            ALPHA_PROXY_CONSUMER_RATE_LIMIT_RPS='0',ALPHA_PROXY_PARALLEL_LIMIT='256',ALPHA_PROXY_SESSION_CAPACITY='1000000',
            GOMAXPROCS=str(min(4,os.cpu_count() or 4)))
        spawn([str(binary)],root,env,'http')
        opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
        for _ in range(100):
            if any(child.poll() is not None for child in children):
                raise RuntimeError('A service exited during startup; inspect runtime logs')
            try:
                with opener.open(f'http://127.0.0.1:{args.http_port}/readyz',timeout=.5) as response:
                    if response.status==200:break
            except OSError:
                time.sleep(.1)
        else:
            raise RuntimeError('HTTP readiness timeout')
        info=dict(supervisor_pid=os.getpid(),child_pids=[c.pid for c in children],http_url=f'http://127.0.0.1:{args.http_port}',
            systems_file=str(auth),grpc_targets=targets,metrics_urls=[f'http://127.0.0.1:{args.metrics_base+i}' for i in range(args.replicas)],
            backend=args.backend,auth_mode=args.auth_mode,replicas=args.replicas,quality_workers_per_replica=args.quality_workers,
            spacy_workers_per_replica=args.spacy_workers,routing_budget_ms=args.routing_budget_ms,
            started_at=time.time(),python=sys.executable,repository=str(root))
        (runtime/'run.json').write_text(json.dumps(info,indent=2)+'\n')
        (runtime/'supervisor.pid').write_text(str(os.getpid())+'\n')
        print(json.dumps(info),flush=True)
        while not stopping:
            if any(child.poll() is not None for child in children):raise RuntimeError('A service exited; inspect runtime logs')
            time.sleep(0.25)
    finally:
        # Stop HTTP first, then ML, giving each a bounded graceful shutdown.
        for child in reversed(children):
            if child.poll() is None:child.terminate()
            try:child.wait(timeout=8)
            except subprocess.TimeoutExpired:child.kill();child.wait()
        for log in logs:log.close()
        (runtime/'supervisor.pid').unlink(missing_ok=True)


if __name__=='__main__':main()
