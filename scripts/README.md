# Qubic Node Installers

Setup scripts for [Bob Node](https://github.com/qubic/core-bob) and [Lite Node](https://github.com/qubic/core-lite) on the Qubic network.

| Node | Description |
|------|-------------|
| **Bob** | Tickchain indexer |
| **Lite** | Lightweight Qubic Core for Linux (no UEFI needed) |

---

## Bob Node

### Requirements

| Component | Minimum |
|-----------|---------|
| RAM | 16 GB |
| CPU | 4+ threads (AVX2) |
| Disk | 100 GB SSD |

### Install

```bash
wget -O bob-install.sh https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/bob-install.sh && chmod +x bob-install.sh && ./bob-install.sh
```

The script will prompt for:
- Node seed
- Node alias
- Peers (auto-fetched if left empty)

### CLI Mode

```bash
./bob-install.sh docker --node-seed YOUR_SEED --node-alias YOUR_ALIAS
```

| Option | Default | Description |
|--------|---------|-------------|
| `--node-seed` | required | Node identity seed |
| `--node-alias` | required | Node alias name |
| `--peers` | auto | Peer IPs (ip:port,ip:port) |
| `--threads` | 0 (auto) | Max threads |
| `--rpc-port` | 40420 | REST API port |
| `--server-port` | 21842 | P2P port |
| `--data-dir` | /opt/qubic-bob | Install directory |

### Management

```bash
/opt/qubic-bob/bob-install.sh status    # container status
/opt/qubic-bob/bob-install.sh logs      # live logs
/opt/qubic-bob/bob-install.sh stop      # stop
/opt/qubic-bob/bob-install.sh start     # start
/opt/qubic-bob/bob-install.sh restart   # restart
/opt/qubic-bob/bob-install.sh update    # pull latest + restart
```

### Uninstall

```bash
/opt/qubic-bob/bob-install.sh uninstall
```

---

## Lite Node

### Requirements

| Component | Mainnet |
|-----------|---------|
| RAM | 64 GB |
| CPU | 8+ threads AVX2/AVX512 (AMD 7950x recommended) |
| Disk | 500 GB SSD |
| Network | 1 Gbit/s |

### Install

```bash
wget -O lite-install.sh https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/lite-install.sh && chmod +x lite-install.sh && ./lite-install.sh
```

The script will prompt for:
- Operator seed
- Operator alias
- Max processors (default: 8)
- Peers (auto-fetched if left empty)

### CLI Mode

```bash
./lite-install.sh docker --operator-seed YOUR_SEED --operator-alias YOUR_ALIAS
```

| Option | Default | Description |
|--------|---------|-------------|
| `--operator-seed` | required | Operator identity seed |
| `--operator-alias` | required | Operator alias name |
| `--peers` | auto | Peer IPs (ip1,ip2) |
| `--port` | 21841 | P2P port |
| `--http-port` | 41841 | HTTP/RPC port |
| `--data-dir` | /opt/qubic-lite | Install directory |
| `--avx512` | off | Enable AVX-512 |
| `--epoch` | auto | Target specific epoch |
| `--no-epoch` | off | Skip epoch data download |
| `--processors` | 8 | Max runtime processors |
| `--threads` | auto | Threads usage |

### Management

```bash
/opt/qubic-lite/lite-install.sh status    # container status
/opt/qubic-lite/lite-install.sh logs      # live logs
/opt/qubic-lite/lite-install.sh stop      # stop
/opt/qubic-lite/lite-install.sh start     # start
/opt/qubic-lite/lite-install.sh restart   # restart
/opt/qubic-lite/lite-install.sh update    # rebuild + restart
```

### Uninstall

```bash
/opt/qubic-lite/lite-install.sh uninstall
```

---

## Troubleshooting

### Bob Node

**Container won't start or exits immediately**
```bash
docker logs qubic-bob              # check error messages
cat /opt/qubic-bob/bob.json        # verify config is valid JSON
```

**Not syncing / no peers**
```bash
/opt/qubic-bob/bob-install.sh logs    # check for connection errors
```
Verify peers in `bob.json` are online at [app.qubic.li/network/live](https://app.qubic.li/network/live)

**API not reachable**
```bash
curl http://localhost:40420/status    # test locally first
sudo ufw status                       # check if port is blocked
```

### Lite Node

**Container won't start**
```bash
docker logs qubic-lite             # check error messages
```

**Node stops ticking after restart**
```bash
rm /opt/qubic-lite/data/system     # delete system file and restart
/opt/qubic-lite/lite-install.sh restart
```

**Won't sync / stuck**
- Verify epoch data exists in `/opt/qubic-lite/data/`
- Check peers are online at [app.qubic.li/network/live](https://app.qubic.li/network/live)
- Download latest epoch from [storage.qubic.li/network](https://storage.qubic.li/network/)

**Out of memory**
- Core lite requires 64GB RAM minimum


