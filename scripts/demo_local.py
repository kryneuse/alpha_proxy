"""Verify masking, retry and exact demasking through the live /process API."""
import argparse,json,urllib.request,uuid
from pathlib import Path
p=argparse.ArgumentParser();p.add_argument('--runtime-dir',default='.runtime/adaptive');args=p.parse_args()
root=Path(__file__).resolve().parents[1];info=json.loads((root/args.runtime_dir/'run.json').read_text())
key=json.loads(Path(info['systems_file']).read_text())[0]['api_key']
opener=urllib.request.build_opener(urllib.request.ProxyHandler({}))
text='👩‍💻 Иванов Пётр Сергеевич, телефон +7 999 123-45-67, email example@example.com. Адрес: Москва, ул. Тестовая, д. 7, кв. 12.'
pid='demo-'+uuid.uuid4().hex

def call(payload):
    req=urllib.request.Request(info['http_url']+'/process',data=json.dumps(dict(payload=payload,payload_id=pid)).encode(),headers={'Content-Type':'application/json','X-API-Key':key})
    with opener.open(req,timeout=10) as response:return json.load(response)['result']
masked=call(text);retry=call(text);restored=call(masked)
assert masked!=text and retry==masked and restored==text
print(json.dumps(dict(masked=masked,exact_round_trip=True,retry_idempotent=True),ensure_ascii=False,indent=2))
