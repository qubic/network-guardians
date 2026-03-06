import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useApi } from '../hooks/useApi'
import { Node } from '../types'
import Card from '../components/Card'
import Badge from '../components/Badge'
import Skeleton from '../components/Skeleton'

function truncateOperator(operator: string): string {
  if (operator.length <= 16) return operator
  return `${operator.slice(0, 8)}...${operator.slice(-8)}`
}

export default function Nodes() {
  const { data: nodes, loading, error } = useApi<Node[]>('/api/v1/nodes', 30000)
  const [filter, setFilter] = useState<'all' | 'lite' | 'bob'>('all')
  const [search, setSearch] = useState('')
  const [hideFlagged, setHideFlagged] = useState(true)
  const [hideOffline, setHideOffline] = useState(true)

  const filteredNodes = nodes
    ?.filter((node) => {
      if (filter !== 'all' && node.type !== filter) return false
      if (hideFlagged && node.flagged) return false
      if (hideOffline && !node.flagged && !node.lastSuccess) return false
      if (search) {
        const searchLower = search.toLowerCase()
        return (
          node.operator.toLowerCase().includes(searchLower) ||
          node.alias?.toLowerCase().includes(searchLower)
        )
      }
      return true
    })
    .sort((a, b) => {
      // flagged nodes always at the bottom
      if (a.flagged !== b.flagged) return a.flagged ? 1 : -1

      // reward > score > sync > uptime > checks
      const rewardDiff = (b.liveScore?.estimatedReward ?? 0) - (a.liveScore?.estimatedReward ?? 0)
      if (rewardDiff !== 0) return rewardDiff

      const scoreDiff = (b.liveScore?.finalScore ?? 0) - (a.liveScore?.finalScore ?? 0)
      if (scoreDiff !== 0) return scoreDiff

      const syncDiff = (b.liveScore?.syncScore ?? 0) - (a.liveScore?.syncScore ?? 0)
      if (syncDiff !== 0) return syncDiff

      const uptimeDiff = (b.liveScore?.uptimeScore ?? 0) - (a.liveScore?.uptimeScore ?? 0)
      if (uptimeDiff !== 0) return uptimeDiff

      return (b.successfulChecks ?? 0) - (a.successfulChecks ?? 0)
    })

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div>
        <h1 className="font-space text-24 font-bold text-white">Nodes</h1>
        <p className="mt-1 text-14 text-gray-50">
          All registered nodes with live scores
        </p>
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div className="flex gap-2">
          {(['all', 'lite', 'bob'] as const).map((type) => (
            <button
              key={type}
              onClick={() => setFilter(type)}
              className={`rounded-lg px-4 py-2 text-sm font-medium capitalize transition-colors ${
                filter === type
                  ? 'bg-primary-30 text-primary-80'
                  : 'bg-primary-60 text-gray-50 hover:text-white'
              }`}
            >
              {type}
            </button>
          ))}
        </div>
        <div className="flex items-center gap-3">
          <button
            onClick={() => setHideFlagged(!hideFlagged)}
            className={`rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
              hideFlagged
                ? 'bg-primary-30/15 text-primary-30 ring-1 ring-primary-30/30'
                : 'bg-primary-60 text-gray-50 hover:text-white'
            }`}
          >
            {hideFlagged ? 'Flagged Hidden' : 'Show Flagged'}
          </button>
          <button
            onClick={() => setHideOffline(!hideOffline)}
            className={`rounded-lg px-3 py-2 text-sm font-medium transition-colors ${
              hideOffline
                ? 'bg-primary-30/15 text-primary-30 ring-1 ring-primary-30/30'
                : 'bg-primary-60 text-gray-50 hover:text-white'
            }`}
          >
            {hideOffline ? 'Offline Hidden' : 'Show Offline'}
          </button>
          <input
            type="text"
            placeholder="Search by operator or alias..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="w-80 rounded-lg border border-primary-60 bg-primary-70 px-4 py-2 text-sm text-white placeholder-gray-50 focus:border-primary-30 focus:outline-none focus:ring-1 focus:ring-primary-30"
          />
        </div>
      </div>

      {/* Nodes Table */}
      <Card className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full table-fixed">
            <thead>
              <tr className="border-b border-primary-60 bg-primary-80">
                <th className="w-[30%] px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-50">
                  Operator
                </th>
                <th className="w-[10%] px-3 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-50">
                  Type
                </th>
                <th className="w-[12%] px-3 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-50">
                  Status
                </th>
                <th className="w-[12%] px-3 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Checks
                </th>
                <th className="w-[12%] px-3 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Uptime
                </th>
                <th className="w-[12%] px-3 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Sync
                </th>
                <th className="w-[12%] px-3 py-3 text-right text-xs font-medium uppercase tracking-wider text-gray-50">
                  Score
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-primary-60">
              {loading ? (
                Array.from({ length: 10 }).map((_, i) => (
                  <tr key={i}>
                    <td className="px-4 py-3">
                      <Skeleton className="h-5 w-40" />
                    </td>
                    <td className="px-3 py-3 text-center">
                      <Skeleton className="mx-auto h-5 w-12" />
                    </td>
                    <td className="px-3 py-3 text-center">
                      <Skeleton className="mx-auto h-5 w-14" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-16" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-12" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-12" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-12" />
                    </td>
                  </tr>
                ))
              ) : error ? (
                <tr>
                  <td colSpan={7} className="px-4 py-8 text-center text-error-40">
                    Error loading nodes: {error}
                  </td>
                </tr>
              ) : filteredNodes?.length ? (
                filteredNodes.map((node) => (
                  <tr
                    key={`${node.operator}-${node.type}`}
                    className="transition-colors hover:bg-primary-60/50"
                  >
                    <td className="px-4 py-3">
                      <Link
                        to={`/nodes/${node.operator}/${node.type}`}
                        className="block"
                      >
                        {node.alias && (
                          <span className="text-sm text-primary-30">{node.alias}</span>
                        )}
                        <span className={`block font-space text-xs tracking-wide ${node.alias ? 'text-gray-50' : 'text-sm text-primary-30'}`}>
                          {truncateOperator(node.operator)}
                        </span>
                      </Link>
                    </td>
                    <td className="whitespace-nowrap px-3 py-3">
                      <div className="flex justify-center">
                        <Badge variant={node.type === 'lite' ? 'info' : 'warning'}>
                          {node.type.charAt(0).toUpperCase() + node.type.slice(1)}
                        </Badge>
                      </div>
                    </td>
                    <td className="whitespace-nowrap px-3 py-3">
                      <div className="flex justify-center">
                        {node.flagged ? (
                          <Badge variant="error">Flagged</Badge>
                        ) : node.lastSuccess ? (
                          <Badge variant="success">Online</Badge>
                        ) : (
                          <Badge variant="error">Offline</Badge>
                        )}
                      </div>
                    </td>
                    <td className="whitespace-nowrap px-3 py-3 text-right text-sm text-white">
                      {node.successfulChecks ?? 0}/{node.totalChecks ?? 0}
                    </td>
                    <td className={`whitespace-nowrap px-3 py-3 text-right text-sm ${
                      (node.liveScore?.uptimeScore ?? 0) < 70
                        ? 'text-red-500'
                        : (node.liveScore?.uptimeScore ?? 0) >= 99.95
                        ? 'text-green-500'
                        : 'text-yellow-500'
                    }`}>
                      {(node.liveScore?.uptimeScore ?? 0).toFixed(1)}%
                    </td>
                    <td className={`whitespace-nowrap px-3 py-3 text-right text-sm ${
                      (node.liveScore?.syncScore ?? 0) < 40
                        ? 'text-red-500'
                        : (node.liveScore?.syncScore ?? 0) >= 99.95
                        ? 'text-green-500'
                        : 'text-yellow-500'
                    }`}>
                      {(node.liveScore?.syncScore ?? 0).toFixed(1)}%
                    </td>
                    <td className={`whitespace-nowrap px-3 py-3 text-right text-sm font-medium ${
                      (node.liveScore?.finalScore ?? 0) < 50
                        ? 'text-red-500'
                        : (node.liveScore?.finalScore ?? 0) >= 99.95
                        ? 'text-green-500'
                        : 'text-yellow-500'
                    }`}>
                      {(node.liveScore?.finalScore ?? 0).toFixed(1)}%
                    </td>
                  </tr>
                ))
              ) : (
                <tr>
                  <td
                    colSpan={7}
                    className="px-4 py-8 text-center text-gray-50"
                  >
                    {nodes ? 'No nodes match your filter' : 'No nodes found'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>

      {/* Summary */}
      {filteredNodes && (
        <p className="text-sm text-gray-50">
          Showing {filteredNodes.length} of {nodes?.length} nodes
        </p>
      )}
    </div>
  )
}
