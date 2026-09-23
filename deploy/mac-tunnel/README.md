# Reverse SSH tunnel

Route: server:18080 -> systemd socket proxy -> server 127.0.0.1:18081 -> SSH -> Mac 127.0.0.1:8080.
Uses the alpha4 alias in ~/.ssh/config. SSH keys are not included. Host verification is enabled and agent forwarding is disabled.

Install the .socket and .service files in /etc/systemd/system on the server, then run:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now alpha-proxy-mac.socket
```

With the local API running, start the tunnel on the Mac:

```bash
python3 deploy/mac-tunnel/tunnel.py start
python3 deploy/mac-tunnel/tunnel.py status
python3 deploy/mac-tunnel/tunnel.py stop
```

The supervisor reconnects after SSH disconnects. State, PID and logs live in .runtime/alpha4-pii-tunnel/. Start again after a Mac reboot. The Mac must remain awake and connected. Stop any previous tunnel before starting this copy on the same remote port.

The public API is http://<server-address>:18080/process. Server-to-Mac traffic uses SSH encryption; public client-to-server traffic is HTTP without TLS.

For monitoring, set PROMETHEUS_CONFIG=./prometheus/mac-tunnel.yml in monitoring/.env and recreate Prometheus using its Compose file. The host.docker.internal alias resolves to the Docker host gateway. This configuration collects HTTP metrics only; individual ML replica metrics are separate.

To close the public listener:

```bash
sudo systemctl disable --now alpha-proxy-mac.socket
sudo systemctl stop alpha-proxy-mac.service
```
