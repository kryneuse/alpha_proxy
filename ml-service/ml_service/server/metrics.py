"""Small localhost metrics endpoint; no text, IDs or personal data are exported."""
import json
import threading
from http.server import BaseHTTPRequestHandler, HTTPServer


def render_prometheus(snapshot):
    lines = ["# TYPE pii_ml_degraded gauge",
             f"pii_ml_degraded {int(snapshot['degraded'])}"]
    for field in ("pending", "running", "capacity", "estimated_ms_per_full_chunk", "queue_ms_sum", "compute_ms_sum"):
        for backend, value in snapshot[field].items():
            lines.append(f'pii_ml_{field}{{backend="{backend}"}} {value}')
    for name, value in sorted(snapshot["counters"].items()):
        lines.append(f"pii_ml_{name}_total {value}")
    return "\n".join(lines) + "\n"


def start_metrics(scheduler, host, port):
    class Handler(BaseHTTPRequestHandler):
        def do_GET(self):
            if self.path == "/metrics":
                body = render_prometheus(scheduler.snapshot()).encode()
                kind = "text/plain; version=0.0.4; charset=utf-8"
            elif self.path == "/stats":
                body = json.dumps(scheduler.snapshot()).encode()
                kind = "application/json"
            else:
                self.send_error(404)
                return
            self.send_response(200)
            self.send_header("Content-Type", kind)
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def setup(self):
            super().setup()
            self.connection.settimeout(2)

        def log_message(self, *args):
            pass

    server = HTTPServer((host, port), Handler)
    threading.Thread(target=server.serve_forever, name="pii-metrics", daemon=True).start()
    return server
