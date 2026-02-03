# Qubic Network Guardians

An incentive system designed to strengthen the Qubic network by encouraging the operation of lightweight nodes and increasing overall decentralization and resilience.

## Problem Statement

The current Qubic network relies heavily on high-resource bare metal nodes with very large memory requirements. This limits the number of operators, reduces redundancy, and increases centralization risk.

## Solution

Network Guardians introduces a monitoring, scoring, and reward system for node operators. Operators run lightweight nodes (Bob or Lite), get automatically discovered and monitored, earn points based on performance metrics, and receive weekly QU rewards proportional to their contribution.

**Dashboard:** [guardians.qubic.org](https://guardians.qubic.org/nodes) - Track your node status, scores, and leaderboard rankings.

---

## How to Participate

### Get an Operator Seed

Before installing a node, you need an operator seed (your node's identity). Create a new wallet at [wallet.qubic.org](https://wallet.qubic.org) and use its seed as your operator seed.

**Important:** Use a dedicated wallet for each node. Do not reuse your main wallet seed.

### Choose Your Node Type

| Node | Description | RAM | CPU | Disk | Network |
|------|-------------|-----|-----|------|---------|
| **Bob** | Tickchain indexer | 16 GB | 4+ threads (AVX2) | 100 GB SSD | 1 Gbit/s |
| **Lite** | Lightweight Qubic Core | 64 GB | 8+ threads AVX2/AVX512 (AMD 7950x recommended) | 500 GB SSD | 1 Gbit/s |

### Install Bob Node

```bash
wget -O bob-install.sh https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/bob-install.sh && chmod +x bob-install.sh && ./bob-install.sh
```

The script prompts for:
- **Node seed** - Your operator identity seed
- **Node alias** - Display name for your node
- **Peers** - Auto-fetched if left empty

Manage your node by running `/opt/qubic-bob/bob-install.sh` without arguments to enter interactive mode, or use CLI commands:
```bash
/opt/qubic-bob/bob-install.sh status    # Container status
/opt/qubic-bob/bob-install.sh logs      # Live logs
/opt/qubic-bob/bob-install.sh start     # Start node
/opt/qubic-bob/bob-install.sh stop      # Stop node
/opt/qubic-bob/bob-install.sh restart   # Restart node
/opt/qubic-bob/bob-install.sh update    # Pull latest & restart
/opt/qubic-bob/bob-install.sh uninstall # Remove node
```

### Install Lite Node

```bash
wget -O lite-install.sh https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/lite-install.sh && chmod +x lite-install.sh && ./lite-install.sh
```

The script prompts for:
- **Operator seed** - Your operator identity seed
- **Operator alias** - Display name for your node
- **Max processors** - Default: 8
- **Peers** - Auto-fetched if left empty

Manage your node by running `/opt/qubic-lite/lite-install.sh` without arguments to enter interactive mode, or use CLI commands:
```bash
/opt/qubic-lite/lite-install.sh status    # Container status
/opt/qubic-lite/lite-install.sh logs      # Live logs
/opt/qubic-lite/lite-install.sh start     # Start node
/opt/qubic-lite/lite-install.sh stop      # Stop node
/opt/qubic-lite/lite-install.sh restart   # Restart node
/opt/qubic-lite/lite-install.sh update    # Rebuild & restart
/opt/qubic-lite/lite-install.sh uninstall # Remove node
```

For detailed installation options and troubleshooting, see [scripts/README.md](scripts/README.md).

---

## Rules and Eligibility

### Participation Rules

| Rule | Description |
|------|-------------|
| **One operator ID = one node** | Each operator ID can only be associated with one node. To run both a Bob and Lite node, you need two different operator IDs. |
| **One node per IP address** | Each IP address can only host one node (regardless of type). |
| **IP change allowed** | You can change your node's IP address (e.g., moving to a new server) and your rewards will carry over, but only if you keep the same node type. Make sure your previous node is offline before starting the new one. |
| **No node type changes** | If you switch from Bob to Lite (or vice versa) mid-epoch with the same operator ID, the previous node is flagged and loses all accumulated points. |
| **Commit for the full epoch** | Start your node at the beginning of an epoch and keep it running until the end to maximize rewards. |

### Eligibility Thresholds

To receive rewards at epoch end, your node must meet **all** of these thresholds:

| Requirement | Threshold | Description |
|-------------|-----------|-------------|
| **Minimum Checks** | 1,500 | Total checks received during the epoch |
| **Uptime Score** | ≥ 70% | Percentage of successful checks |
| **Sync Score** | ≥ 50% | Average synchronization score |

### Automatic Flagging (Disqualification)

Nodes are automatically flagged and excluded from rewards for:

| Flag Reason | What Happens |
|-------------|--------------|
| **Duplicate IP** | Running multiple nodes on the same IP. Only the most recent node is eligible. |
| **Duplicate Operator** | Running multiple nodes of the same type with one operator ID. Only the most recent node is eligible. |
| **Node Type Change** | Switching from Bob to Lite (or vice versa) mid-epoch with the same operator ID. The old node is flagged. |

**Important:** When flagged, your previous node loses all accumulated points for that epoch. The newest registration starts fresh.

### Manual Flagging

Nodes may also be manually flagged for:
- Relay or proxy node abuse
- Network manipulation
- Other violations of fair participation

---

## Scoring System

Nodes are continuously checked and scored on two metrics:

### Uptime Score (60% weight)
```
Uptime Score = (Successful Checks / Total Checks) × 100
```
Measures how often your node responds successfully to health checks.

### Sync Score (40% weight)
```
If node_tick >= reference_tick: Score = 100
If ticks_behind <= 100: Score = 100  (buffer zone)
If ticks_behind > 100: Score = 100 - ((ticks_behind - 100) × 0.15)
```
Measures how well-synchronized your node is with the network. Nodes within 100 ticks of the reference receive full marks.

### Final Score
```
Final Score = (Uptime Score × 0.6) + (Sync Score × 0.4)
```

### Reward Points
```
Reward Points = Final Score × Successful Checks
```
Higher uptime and staying in sync results in more reward points.

---

## Reward Distribution

### Epoch Cycle
- **Duration:** 1 week (Wednesday to Wednesday)
- **Transition:** Every Wednesday at 12:00 UTC
- **Grace Period:** 1 hour after transition (12:00-13:00 UTC) - no checks during this time

### Reward Pools

The total reward pool is split between node types:

| Pool | Percentage |
|------|------------|
| **Lite Nodes** | 80% |
| **Bob Nodes** | 20% |

### Reward Calculation

Within each pool, rewards are distributed proportionally based on reward points:

```
Your Reward = (Your Reward Points / Total Pool Reward Points) × Pool Amount
```

---

## Long Term Vision

Transition the system to a fully on-chain model using a smart contract funded reward pool. Network statistics would be delivered through OM and used for automated reward distribution.

---

## FAQ

**How do I check if my node is being monitored?**

Visit the [Guardians Dashboard](https://guardians.qubic.org/nodes) and search for your operator ID or alias. If your node appears, it's being tracked.

**When are rewards distributed?**

Rewards are calculated at the end of each epoch (every Wednesday at 12:00 UTC). The reward amounts are recorded and distributed according to the current distribution process.

**Why is my node flagged?**

Check the dashboard for your node's flag reason. Common causes: duplicate IP address, duplicate operator ID, or switching node types mid-epoch.

**Can I run both a Bob and Lite node?**

Yes, but you need a different operator ID for each node. Each operator ID can only be associated with one node type.

**What happens during the grace period?**

During the 1-hour grace period (Wednesday 12:00-13:00 UTC), no checks are performed. This allows the network to stabilize after epoch transition. Your node won't lose points during this time.

**My node is online but has low uptime score. Why?**

Failed checks count against your uptime. Common causes: firewall blocking the checker ports (Bob: 40420, Lite: 41841), node too slow, or response timeouts.

---

## Support

Need help? Join the Qubic Discord: [discord.gg/G8qxTddTec](https://discord.gg/G8qxTddTec)

---

## Links

| Resource | URL |
|----------|-----|
| Guardians Dashboard | [guardians.qubic.org](https://guardians.qubic.org/nodes) |
| Bob Node | [GitHub](https://github.com/qubic/core-bob) |
| Lite Node | [GitHub](https://github.com/qubic/core-lite) |
| Epoch Data | [storage.qubic.li](https://storage.qubic.li/network/) |
