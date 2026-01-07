# Qubic Network Guardians

## Purpose
Qubic Network Guardians is an incentive system designed to strengthen the Qubic network by encouraging the operation of lightweight nodes and increasing overall decentralization and resilience.

## Problem Statement
The current Qubic network relies heavily on high resource bare metal nodes with very large memory requirements. This limits the number of operators, reduces redundancy, and increases centralization risk.

## Core Idea
Reward community members for running and maintaining lighter node types such as core lite and bob. These nodes improve network availability, data redundancy, and accessibility without requiring extreme hardware.

## Solution Overview
Network Guardians introduces a monitoring, scoring, and reward system for node operators. Nodes are evaluated continuously based on measurable performance metrics. Operators earn points, appear on a public leaderboard, and receive epoch based QU rewards proportional to their contribution.

### How It Works

#### Node Participation
Operators run a bob or core lite node and configure it with an operator identity and optional display name.

#### Discovery and Monitoring
Nodes are automatically discovered and monitored using network crawling and node announcements.

#### Scoring System
Points are assigned using weighted criteria:

Uptime: 50 percent
Synchronization status: 30 percent
Data correctness and validity: 20 percent

#### Leaderboard
All participating operators are ranked transparently based on their score.

#### Rewards
QU rewards are distributed at the end of each epoch according to the operator score.
The initial phase operates without a smart contract. A later phase plans full smart contract based reward distribution.

#### Anti Abuse Measures
Mechanisms are planned to prevent relay or proxy nodes and to limit multiple nodes per operator identity.

### Technical Requirements

#### bob node
Minimum 16 GB RAM, 4 CPU cores with AVX2, stable internet connection.

#### core lite node
Minimum 64 GB RAM, 8 CPU cores with AVX2 or AVX512, 1 Gbps network recommended.

## Long Term Vision
Transition the system to a fully on chain model using a smart contract funded reward pool. Network statistics would be delivered through OM and used for automated reward distribution.

## Summary
Qubic Network Guardians creates clear economic incentives for running lightweight nodes. It improves decentralization, increases redundancy, and strengthens network reliability while lowering the barrier to participation for the community.
