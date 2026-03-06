import { useState } from 'react'
import Card from '../components/Card'

export default function GetStarted() {
  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div>
        <h1 className="font-space text-24 font-bold text-white">Get Started</h1>
        <p className="mt-1 text-14 text-gray-50">
          Everything you need to set up a Qubic Guardian node and start earning rewards.
        </p>
      </div>

      {/* Hero / Intro */}
      <Card className="p-6">
        <p className="text-15 leading-relaxed text-gray-300">
          Network Guardians strengthens the Qubic network by rewarding operators of lightweight nodes.
          Run a <span className="text-primary-30">Bob</span> or <span className="text-primary-30">Lite</span> node,
          get automatically discovered and monitored, earn points based on performance, and receive
          weekly <span className="text-primary-30">QU rewards</span> proportional to your contribution.
        </p>
      </Card>

      {/* Step 1 — Operator Seed */}
      <Section title="1. Get an Operator Seed">
        <p className="text-gray-300">
          Before installing a node you need an operator seed — your node's identity. Create a new wallet at{' '}
          <a href="https://wallet.qubic.org" target="_blank" rel="noopener noreferrer" className="text-primary-30 hover:underline">
            wallet.qubic.org
          </a>{' '}
          and use its seed as your operator seed.
        </p>
        <p className="mt-3 text-sm text-yellow-400">
          Important: Use a dedicated wallet for each node. Do not reuse your main wallet seed.
        </p>
      </Section>

      {/* Step 2 — Choose Node Type */}
      <Section title="2. Choose Your Node Type">
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-primary-60 text-gray-50">
                <th className="pb-3 pr-4 font-medium">Node</th>
                <th className="pb-3 pr-4 font-medium">Description</th>
                <th className="pb-3 pr-4 font-medium">RAM</th>
                <th className="pb-3 pr-4 font-medium">CPU</th>
                <th className="pb-3 pr-4 font-medium">Disk</th>
                <th className="pb-3 font-medium">Network</th>
              </tr>
            </thead>
            <tbody className="text-gray-300">
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-amber-400">Bob</td>
                <td className="py-3 pr-4">Tickchain indexer</td>
                <td className="py-3 pr-4">16 GB</td>
                <td className="py-3 pr-4">4+ threads (AVX2)</td>
                <td className="py-3 pr-4">100 GB SSD</td>
                <td className="py-3">1 Gbit/s</td>
              </tr>
              <tr>
                <td className="py-3 pr-4 font-medium text-primary-30">Lite</td>
                <td className="py-3 pr-4">Lightweight Qubic Core</td>
                <td className="py-3 pr-4">64 GB</td>
                <td className="py-3 pr-4">8+ threads AVX2/AVX512</td>
                <td className="py-3 pr-4">500 GB SSD</td>
                <td className="py-3">1 Gbit/s</td>
              </tr>
            </tbody>
          </table>
        </div>
      </Section>

      {/* Step 3 — Installation */}
      <Section title="3. Installation">
        <div className="space-y-6">
          <div>
            <h4 className="mb-2 font-space text-15 font-semibold text-amber-400">Bob Node</h4>
            <CodeBlock copyable>{`wget -O bob.sh https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/bob.sh && chmod +x bob.sh && ./bob.sh`}</CodeBlock>
            <p className="mt-2 text-sm text-gray-50">
              The script prompts for: node seed, node alias, and peers (auto-fetched if left empty).
            </p>
            <p className="mt-4 mb-2 text-sm text-gray-300">
              Manage your node by running <code className="rounded bg-primary-80 px-1.5 py-0.5 text-primary-20">/opt/qubic-bob/bob.sh</code> without arguments to enter interactive mode, or use CLI commands:
            </p>
            <CodeBlock>{`/opt/qubic-bob/bob.sh status    # Container status
/opt/qubic-bob/bob.sh logs      # Live logs
/opt/qubic-bob/bob.sh start     # Start node
/opt/qubic-bob/bob.sh stop      # Stop node
/opt/qubic-bob/bob.sh restart   # Restart node
/opt/qubic-bob/bob.sh update    # Pull latest & restart
/opt/qubic-bob/bob.sh uninstall # Remove node`}</CodeBlock>
          </div>

          <div>
            <h4 className="mb-2 font-space text-15 font-semibold text-primary-30">Lite Node</h4>
            <CodeBlock copyable>{`wget -O lite.sh https://raw.githubusercontent.com/qubic/network-guardians/main/scripts/lite.sh && chmod +x lite.sh && ./lite.sh`}</CodeBlock>
            <p className="mt-2 text-sm text-gray-50">
              The script prompts for: operator seed, operator alias, max processors (default: 8), and peers (auto-fetched if left empty).
            </p>
            <p className="mt-4 mb-2 text-sm text-gray-300">
              Manage your node by running <code className="rounded bg-primary-80 px-1.5 py-0.5 text-primary-20">/opt/qubic-lite/lite.sh</code> without arguments to enter interactive mode, or use CLI commands:
            </p>
            <CodeBlock>{`/opt/qubic-lite/lite.sh status    # Container status
/opt/qubic-lite/lite.sh logs      # Live logs
/opt/qubic-lite/lite.sh start     # Start node
/opt/qubic-lite/lite.sh stop      # Stop node
/opt/qubic-lite/lite.sh restart   # Restart node
/opt/qubic-lite/lite.sh update    # Rebuild & restart
/opt/qubic-lite/lite.sh uninstall # Remove node`}</CodeBlock>
          </div>
        </div>
      </Section>

      {/* Rules & Eligibility */}
      <Section title="Rules & Eligibility">
        <h4 className="mb-3 font-space text-15 font-semibold text-white">Participation Rules</h4>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-primary-60 text-gray-50">
                <th className="pb-3 pr-4 font-medium">Rule</th>
                <th className="pb-3 font-medium">Description</th>
              </tr>
            </thead>
            <tbody className="text-gray-300">
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">One operator ID = one node</td>
                <td className="py-3">Each operator ID can only be associated with one node. To run both Bob and Lite, use two different operator IDs.</td>
              </tr>
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">One node per IP</td>
                <td className="py-3">Each IP address can only host one node (regardless of type).</td>
              </tr>
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">IP change allowed</td>
                <td className="py-3">You can change your node's IP (e.g., new server) and rewards carry over, but only if you keep the same node type.</td>
              </tr>
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">No type changes</td>
                <td className="py-3">Switching from Bob to Lite (or vice versa) mid-epoch flags the previous node and loses all accumulated points.</td>
              </tr>
              <tr>
                <td className="py-3 pr-4 font-medium text-white">Commit for the full epoch</td>
                <td className="py-3">Start at the beginning of an epoch and keep running until the end to maximize rewards.</td>
              </tr>
            </tbody>
          </table>
        </div>

        <h4 className="mb-3 mt-8 font-space text-15 font-semibold text-white">Eligibility Thresholds</h4>
        <p className="mb-3 text-sm text-gray-50">To receive rewards at epoch end, your node must meet all of these:</p>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-primary-60 text-gray-50">
                <th className="pb-3 pr-4 font-medium">Requirement</th>
                <th className="pb-3 pr-4 font-medium">Threshold</th>
                <th className="pb-3 font-medium">Description</th>
              </tr>
            </thead>
            <tbody className="text-gray-300">
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">Minimum Checks</td>
                <td className="py-3 pr-4 text-primary-30">1,500</td>
                <td className="py-3">Total checks received during the epoch</td>
              </tr>
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">Uptime Score</td>
                <td className="py-3 pr-4 text-primary-30">&ge; 70%</td>
                <td className="py-3">Percentage of successful checks</td>
              </tr>
              <tr>
                <td className="py-3 pr-4 font-medium text-white">Sync Score</td>
                <td className="py-3 pr-4 text-primary-30">&ge; 50%</td>
                <td className="py-3">Average synchronization score</td>
              </tr>
            </tbody>
          </table>
        </div>

        <h4 className="mb-3 mt-8 font-space text-15 font-semibold text-white">Automatic Flagging (Disqualification)</h4>
        <div className="overflow-x-auto">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-primary-60 text-gray-50">
                <th className="pb-3 pr-4 font-medium">Flag Reason</th>
                <th className="pb-3 font-medium">What Happens</th>
              </tr>
            </thead>
            <tbody className="text-gray-300">
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">Duplicate IP</td>
                <td className="py-3">Multiple nodes on the same IP. Only the most recent node is eligible.</td>
              </tr>
              <tr className="border-b border-primary-60/50">
                <td className="py-3 pr-4 font-medium text-white">Duplicate Operator</td>
                <td className="py-3">Multiple nodes of the same type with one operator ID. Only the most recent is eligible.</td>
              </tr>
              <tr>
                <td className="py-3 pr-4 font-medium text-white">Node Type Change</td>
                <td className="py-3">Switching type mid-epoch. The old node is flagged and loses all points.</td>
              </tr>
            </tbody>
          </table>
        </div>
      </Section>

      {/* Reward Distribution */}
      <Section title="Reward Distribution">
        <div className="space-y-6">
          <div>
            <h4 className="mb-3 font-space text-15 font-semibold text-white">Epoch Cycle</h4>
            <ul className="space-y-1 text-sm text-gray-300">
              <li><span className="text-gray-50">Duration:</span> 1 week (Wednesday to Wednesday)</li>
              <li><span className="text-gray-50">Transition:</span> Every Wednesday at 12:00 UTC</li>
              <li><span className="text-gray-50">Grace Period:</span> 1 hour after transition (12:00–13:00 UTC) — no checks during this time</li>
            </ul>
          </div>

          <div>
            <h4 className="mb-3 font-space text-15 font-semibold text-white">Reward Pools</h4>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-primary-60 text-gray-50">
                    <th className="pb-3 pr-4 font-medium">Pool</th>
                    <th className="pb-3 font-medium">Percentage</th>
                  </tr>
                </thead>
                <tbody className="text-gray-300">
                  <tr className="border-b border-primary-60/50">
                    <td className="py-3 pr-4 font-medium text-primary-30">Lite Nodes</td>
                    <td className="py-3 text-primary-30">80%</td>
                  </tr>
                  <tr>
                    <td className="py-3 pr-4 font-medium text-amber-400">Bob Nodes</td>
                    <td className="py-3 text-amber-400">20%</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>

          <div>
            <h4 className="mb-2 font-space text-15 font-semibold text-white">Reward Calculation</h4>
            <CodeBlock>{`Your Reward = (Your Reward Points / Total Pool Reward Points) × Pool Amount`}</CodeBlock>
          </div>
        </div>
      </Section>

      {/* FAQ */}
      <Section title="FAQ">
        <div className="space-y-6">
          <FaqItem question="How do I check if my node is being monitored?">
            Visit the{' '}
            <a href="https://guardians.qubic.org/nodes" target="_blank" rel="noopener noreferrer" className="text-primary-30 hover:underline">
              Guardians Dashboard
            </a>{' '}
            and search for your operator ID or alias. If your node appears, it's being tracked.
          </FaqItem>
          <FaqItem question="When are rewards distributed?">
            Rewards are calculated at the end of each epoch (every Wednesday at 12:00 UTC). The reward amounts are recorded and distributed according to the current distribution process.
          </FaqItem>
          <FaqItem question="Why is my node flagged?">
            Check the dashboard for your node's flag reason. Common causes: duplicate IP address, duplicate operator ID, or switching node types mid-epoch.
          </FaqItem>
          <FaqItem question="Can I run both a Bob and Lite node?">
            Yes, but you need a different operator ID for each node. Each operator ID can only be associated with one node type.
          </FaqItem>
          <FaqItem question="What happens during the grace period?">
            During the 1-hour grace period (Wednesday 12:00–13:00 UTC), no checks are performed. This allows the network to stabilize after epoch transition. Your node won't lose points during this time.
          </FaqItem>
          <FaqItem question="My node is online but has low uptime score. Why?">
            Failed checks count against your uptime. Common causes: firewall blocking required ports, node too slow, or response timeouts.
          </FaqItem>
          <div>
            <h4 className="mb-2 text-15 font-medium text-white">Which ports need to be open?</h4>
            <div className="overflow-x-auto">
              <table className="w-full text-left text-sm">
                <thead>
                  <tr className="border-b border-primary-60 text-gray-50">
                    <th className="pb-3 pr-4 font-medium">Node</th>
                    <th className="pb-3 pr-4 font-medium">Port</th>
                    <th className="pb-3 pr-4 font-medium">Protocol</th>
                    <th className="pb-3 font-medium">Purpose</th>
                  </tr>
                </thead>
                <tbody className="text-gray-300">
                  <tr className="border-b border-primary-60/50">
                    <td className="py-3 pr-4 font-medium text-primary-30">Lite</td>
                    <td className="py-3 pr-4">21841</td>
                    <td className="py-3 pr-4">P2P</td>
                    <td className="py-3">Primary check port — validates tick data and quorum votes for current ticks to verify correctness</td>
                  </tr>
                  <tr className="border-b border-primary-60/50">
                    <td className="py-3 pr-4 font-medium text-primary-30">Lite</td>
                    <td className="py-3 pr-4">41841</td>
                    <td className="py-3 pr-4">HTTP API</td>
                    <td className="py-3">Health check endpoint</td>
                  </tr>
                  <tr className="border-b border-primary-60/50">
                    <td className="py-3 pr-4 font-medium text-amber-400">Bob</td>
                    <td className="py-3 pr-4">40420</td>
                    <td className="py-3 pr-4">HTTP API</td>
                    <td className="py-3">Health check endpoint</td>
                  </tr>
                  <tr>
                    <td className="py-3 pr-4 font-medium text-amber-400">Bob</td>
                    <td className="py-3 pr-4">21842</td>
                    <td className="py-3 pr-4">P2P</td>
                    <td className="py-3">Must be open — queries meaningful data as proof of operation</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
        </div>
      </Section>

      {/* Support */}
      <Card className="p-6 text-center">
        <p className="text-gray-300">
          Need help? Join the Qubic Discord:{' '}
          <a href="https://discord.gg/G8qxTddTec" target="_blank" rel="noopener noreferrer" className="text-primary-30 hover:underline">
            discord.gg/G8qxTddTec
          </a>
        </p>
      </Card>
    </div>
  )
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <Card className="p-6">
      <h3 className="mb-4 font-space text-18 font-semibold text-white">{title}</h3>
      {children}
    </Card>
  )
}

function CodeBlock({ children, copyable }: { children: string; copyable?: boolean }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = () => {
    navigator.clipboard.writeText(children)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="group relative">
      <pre className={`overflow-x-auto rounded-lg bg-primary-80 p-4 font-mono text-sm text-primary-20${copyable ? ' pr-12' : ''}`}>
        {children}
      </pre>
      {copyable && (
        <button
          onClick={handleCopy}
          className="absolute right-2 top-2 rounded p-1.5 text-gray-50 transition-colors hover:bg-primary-60 hover:text-white"
          title="Copy to clipboard"
        >
          {copied ? (
            <svg className="h-4 w-4 text-green-400" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <path strokeLinecap="round" strokeLinejoin="round" d="M5 13l4 4L19 7" />
            </svg>
          ) : (
            <svg className="h-4 w-4" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
              <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
              <path d="M5 15H4a2 2 0 01-2-2V4a2 2 0 012-2h9a2 2 0 012 2v1" />
            </svg>
          )}
        </button>
      )}
    </div>
  )
}

function FaqItem({ question, children }: { question: string; children: React.ReactNode }) {
  return (
    <div>
      <h4 className="mb-2 text-15 font-medium text-white">{question}</h4>
      <p className="text-sm text-gray-300">{children}</p>
    </div>
  )
}
