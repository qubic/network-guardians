# Network Guard

Setup scripts for running [Bob Node](https://github.com/qubic/core-bob) and [Lite Node](https://github.com/qubic/core-lite) on the Qubic network.

## Table of Contents

**Bob Node**

1. [Requirements](#requirements)
2. [Quick Start](#quick-start)
3. [Management](#management)
4. [Ports](#ports)
5. [Cloud Provider Examples](#cloud-provider-examples)
6. [Firewall](#firewall)
7. [Troubleshooting](#troubleshooting)

**Lite Node**

8. [Requirements](#requirements-1)
9. [Quick Start](#quick-start-1)
10. [Management](#management-1)
11. [Ports](#ports-1)
12. [Cloud Provider Examples](#cloud-provider-examples-1)
13. [Firewall](#firewall-1)
14. [Troubleshooting](#troubleshooting-1)

**General**

15. [Links](#links)

---

# Bob Node

Blockchain indexer with REST API for the Qubic network.

## Requirements

| Component | Minimum |
|-----------|---------|
| RAM | 16 GB |
| CPU | 4+ cores |
| Disk | 100 GB SSD |
| Docker | Required |

## Quick Start

```bash
wget -O bob.sh https://raw.githubusercontent.com/Pomm3sgab3l/Network_Guard/main/scripts/bob.sh
chmod +x bob.sh && ./bob.sh
```

The script provides an interactive menu and stores data in `/opt/qubic-bob`.

## Management

```bash
cd /opt/qubic-bob
./bob.sh status     # show status
./bob.sh logs       # view logs
./bob.sh stop       # stop node
./bob.sh start      # start node
./bob.sh restart    # restart node
./bob.sh uninstall  # remove node
```

> **Auto-Update:** Watchtower automatically updates the node when new versions are released.

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 21842 | TCP | P2P (required) |
| 40420 | TCP | REST API |

## Cloud Provider Examples

The following providers have been tested with Bob Node:

- Hetzner Cloud
- Netcup / Hostkey
- OVH / Bare Metal
- AWS EC2
- Google Cloud
- DigitalOcean

> **Note:** These are examples only. We do not guarantee that any provider permits running blockchain nodes. Please check the provider's terms of service before deploying.

### Cloud-Init

Deploy with a single command using [cloud-init](cloud-init/).

## Firewall

```bash
# UFW (Ubuntu)
sudo ufw allow 22/tcp      # SSH
sudo ufw allow 21842/tcp   # P2P
sudo ufw allow 40420/tcp   # API (optional)
sudo ufw enable
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Container exits immediately | Check logs: `docker logs qubic-bob` |
| Not syncing | Wait a few minutes, check logs for peer connections |
| API not reachable | Check firewall, ensure port 40420 is open |

---

# Lite Node

Lightweight Qubic Core that runs on Linux without UEFI.

## Requirements

| Component | Testnet | Mainnet |
|-----------|---------|---------|
| RAM | 16 GB | 64 GB |
| CPU | x86_64 | High-freq AVX2 |
| Disk | - | 500 GB SSD |

## Quick Start

```bash
wget -O lite.sh https://raw.githubusercontent.com/Pomm3sgab3l/Network_Guard/main/scripts/lite.sh
chmod +x lite.sh && ./lite.sh
```

The script provides an interactive menu and stores data in `/opt/qubic-lite`.

## Management

```bash
cd /opt/qubic-lite
./lite.sh status    # show status
./lite.sh logs      # view logs
./lite.sh stop      # stop node
./lite.sh start     # start node
./lite.sh restart   # restart node
./lite.sh uninstall # remove node
```

> **Auto-Update:** Watchtower automatically updates the node when new versions are released.

## Ports

| Port | Protocol | Description |
|------|----------|-------------|
| 21841 | TCP | P2P (required) |
| 41841 | TCP | HTTP API |

## Cloud Provider Examples

The following providers have been tested with Lite Node:

- Hetzner Cloud
- Netcup / Hostkey
- OVH / Bare Metal
- AWS EC2
- Google Cloud
- DigitalOcean

> **Note:** These are examples only. We do not guarantee that any provider permits running blockchain nodes. Please check the provider's terms of service before deploying.

### Cloud-Init

Deploy with a single command using [cloud-init](cloud-init/).

## Firewall

```bash
# UFW (Ubuntu)
sudo ufw allow 22/tcp      # SSH
sudo ufw allow 21841/tcp   # P2P
sudo ufw allow 41841/tcp   # HTTP API
sudo ufw enable
```

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Build fails | Check if AVX2 is supported: `grep avx2 /proc/cpuinfo` |
| Not syncing | Check logs, verify peers are connecting |
| High CPU usage | Normal during sync |

---

## Links

- Bob Node: [qubic/core-bob](https://github.com/qubic/core-bob) | [Docker Hub](https://hub.docker.com/r/qubiccore/bob)
- Lite Node: [qubic/core-lite](https://github.com/qubic/core-lite) | [Docker Hub](https://hub.docker.com/r/qubiccore/lite)
- Qubic: [Website](https://qubic.org) | [Network Dashboard](https://app.qubic.li/network/live)

