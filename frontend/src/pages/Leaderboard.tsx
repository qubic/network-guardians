import { useState, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { useApi } from '../hooks/useApi'
import { LeaderboardResponse, LeaderboardEntry } from '../types'
import Card from '../components/Card'
import Badge from '../components/Badge'
import Skeleton from '../components/Skeleton'

type NodeTypeFilter = 'all' | 'lite' | 'bob'
type SortKey = 'reward' | 'points' | 'score' | 'country'
type SortDir = 'asc' | 'desc'

function truncateOperator(operator: string): string {
  if (operator.length <= 16) return operator
  return `${operator.slice(0, 8)}...${operator.slice(-8)}`
}

function formatNumber(num: number): string {
  return num.toLocaleString()
}

function getSortValue(entry: LeaderboardEntry, key: SortKey): number | string {
  switch (key) {
    case 'reward':
      return entry.liveScore?.estimatedReward ?? 0
    case 'points':
      return entry.liveScore?.rewardPoints ?? 0
    case 'score':
      return entry.liveScore?.finalScore ?? 0
    case 'country':
      return (entry.country ?? '').toLowerCase()
  }
}

function SortIcon({ column, sortKey, sortDir }: { column: SortKey; sortKey: SortKey; sortDir: SortDir }) {
  const active = column === sortKey
  return (
    <span className={`ml-1 inline-block ${active ? 'text-primary-30' : 'text-gray-60'}`}>
      {active ? (sortDir === 'desc' ? '\u25BC' : '\u25B2') : '\u25BC'}
    </span>
  )
}

export default function Leaderboard() {
  const [typeFilter, setTypeFilter] = useState<NodeTypeFilter>('all')
  const [sortKey, setSortKey] = useState<SortKey>('reward')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const [search, setSearch] = useState('')

  const endpoint = typeFilter === 'all'
    ? '/api/v1/leaderboard'
    : `/api/v1/leaderboard?type=${typeFilter}`

  const { data, loading, error } = useApi<LeaderboardResponse>(endpoint, 30000)

  const handleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir(prev => prev === 'desc' ? 'asc' : 'desc')
    } else {
      setSortKey(key)
      setSortDir(key === 'country' ? 'asc' : 'desc')
    }
  }

  const sortedRankings = useMemo(() => {
    if (!data?.rankings?.length) return []

    let filtered = data.rankings
    if (search) {
      const q = search.toLowerCase()
      filtered = filtered.filter(e =>
        e.operator.toLowerCase().includes(q) ||
        (e.alias && e.alias.toLowerCase().includes(q))
      )
    }

    return [...filtered].sort((a, b) => {
      const aVal = getSortValue(a, sortKey)
      const bVal = getSortValue(b, sortKey)

      let cmp: number
      if (typeof aVal === 'string' && typeof bVal === 'string') {
        cmp = aVal.localeCompare(bVal)
      } else {
        cmp = (aVal as number) - (bVal as number)
      }

      if (cmp === 0) {
        // Tiebreak: est. reward desc, then operator asc for stability
        const rewardCmp = (b.liveScore?.estimatedReward ?? 0) - (a.liveScore?.estimatedReward ?? 0)
        if (rewardCmp !== 0) return rewardCmp
        return a.operator.localeCompare(b.operator)
      }

      return sortDir === 'desc' ? -cmp : cmp
    })
  }, [data?.rankings, sortKey, sortDir, search])

  const networkStats = useMemo(() => {
    if (!data?.rankings?.length) return null

    const rankings = data.rankings
    const totalUptime = rankings.reduce((sum, e) => sum + (e.liveScore?.uptimeScore ?? 0), 0)
    const totalSync = rankings.reduce((sum, e) => sum + (e.liveScore?.syncScore ?? 0), 0)
    const totalScore = rankings.reduce((sum, e) => sum + (e.liveScore?.finalScore ?? 0), 0)

    return {
      avgUptime: totalUptime / rankings.length,
      avgSync: totalSync / rankings.length,
      avgScore: totalScore / rankings.length
    }
  }, [data?.rankings])

  const tabs: { key: NodeTypeFilter; label: string; count?: number }[] = [
    { key: 'all', label: 'All', count: data?.info?.eligibleForRewards },
    { key: 'lite', label: 'Lite', count: data?.info?.liteCount },
    { key: 'bob', label: 'Bob', count: data?.info?.bobCount },
  ]

  const thSortable = (label: string, key: SortKey, align: string, width: string) => (
    <th
      className={`${width} cursor-pointer select-none px-3 py-3 text-xs font-medium uppercase tracking-wider text-gray-50 transition-colors hover:text-primary-30 ${align}`}
      onClick={() => handleSort(key)}
    >
      {label}
      <SortIcon column={key} sortKey={sortKey} sortDir={sortDir} />
    </th>
  )

  const colSpan = typeFilter === 'all' ? 7 : 6

  return (
    <div className="space-y-6">
      {/* Page Header */}
      <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="font-space text-24 font-bold text-white">Leaderboard</h1>
          <p className="mt-1 text-14 text-gray-50">
            Current epoch rankings by reward pool
          </p>
        </div>

        {/* Type Filter Tabs */}
        <div className="flex gap-1 rounded-lg bg-primary-70 p-1">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setTypeFilter(tab.key)}
              className={`rounded-md px-4 py-2 text-sm font-medium transition-colors ${
                typeFilter === tab.key
                  ? 'bg-primary-50 text-white'
                  : 'text-gray-50 hover:text-white'
              }`}
            >
              {tab.label}
              {tab.count !== undefined && (
                <span className={`ml-1.5 text-xs ${
                  typeFilter === tab.key ? 'text-gray-30' : 'text-gray-50'
                }`}>
                  ({tab.count})
                </span>
              )}
            </button>
          ))}
        </div>
      </div>

      {/* Pool Info Banner */}
      {data?.pool && (
        <Card className="bg-primary-60/50 p-4">
          <div className="flex flex-wrap items-center gap-6 text-sm">
            <div>
              <span className="text-gray-50">Pool: </span>
              <span className="font-semibold text-primary-30">
                {formatNumber(data.pool.amount)} QUS
              </span>
              {typeFilter !== 'all' && (
                <span className="ml-1 text-gray-50">
                  ({Math.round((data.pool.amount / data.pool.totalPool) * 100)}% of total)
                </span>
              )}
            </div>
            <div>
              <span className="text-gray-50">Epoch: </span>
              <span className="text-white">{data.reference.epoch}</span>
            </div>
            <div>
              <span className="text-gray-50">Nodes: </span>
              <span className="text-white">{data.info.filteredCount}</span>
            </div>
            {networkStats && (
              <>
                <div className="border-l border-primary-50 pl-6">
                  <span className="text-gray-50">Avg Uptime: </span>
                  <span className="text-white">{networkStats.avgUptime.toFixed(1)}%</span>
                </div>
                <div>
                  <span className="text-gray-50">Avg Sync: </span>
                  <span className="text-white">{networkStats.avgSync.toFixed(1)}%</span>
                </div>
                <div>
                  <span className="text-gray-50">Avg Score: </span>
                  <span className="font-semibold text-primary-30">{networkStats.avgScore.toFixed(1)}%</span>
                </div>
              </>
            )}
          </div>
        </Card>
      )}

      {/* Search */}
      <div className="relative">
        <input
          type="text"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Search by operator ID or alias..."
          className="w-full rounded-lg border border-primary-60 bg-primary-70 px-4 py-2.5 pl-10 text-sm text-white placeholder-gray-50 outline-none transition-colors focus:border-primary-40"
        />
        <svg className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-gray-50" fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2}>
          <path strokeLinecap="round" strokeLinejoin="round" d="M21 21l-4.35-4.35M11 19a8 8 0 100-16 8 8 0 000 16z" />
        </svg>
        {search && (
          <button
            onClick={() => setSearch('')}
            className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-50 hover:text-white"
          >
            Clear
          </button>
        )}
      </div>

      {/* Leaderboard Table */}
      <Card className="overflow-hidden">
        <div className="overflow-x-auto">
          <table className="w-full table-fixed">
            <thead>
              <tr className="border-b border-primary-60 bg-primary-80">
                <th className="w-[8%] px-3 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-50">
                  Rank
                </th>
                <th className="w-[32%] px-4 py-3 text-left text-xs font-medium uppercase tracking-wider text-gray-50">
                  Operator
                </th>
                {typeFilter === 'all' && (
                  <th className="w-[10%] px-3 py-3 text-center text-xs font-medium uppercase tracking-wider text-gray-50">
                    Type
                  </th>
                )}
                {thSortable('Country', 'country', 'text-center', 'w-[12%]')}
                {thSortable('Score', 'score', 'text-right', 'w-[12%]')}
                {thSortable('Points', 'points', 'text-right', 'w-[14%]')}
                {thSortable('Est. Reward', 'reward', 'text-right', 'w-[16%]')}
              </tr>
            </thead>
            <tbody className="divide-y divide-primary-60">
              {loading ? (
                Array.from({ length: 20 }).map((_, i) => (
                  <tr key={i}>
                    <td className="px-3 py-3">
                      <Skeleton className="h-5 w-8" />
                    </td>
                    <td className="px-4 py-3">
                      <Skeleton className="h-5 w-40" />
                    </td>
                    {typeFilter === 'all' && (
                      <td className="px-3 py-3 text-center">
                        <Skeleton className="mx-auto h-5 w-12" />
                      </td>
                    )}
                    <td className="px-3 py-3 text-center">
                      <Skeleton className="mx-auto h-5 w-8" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-12" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-16" />
                    </td>
                    <td className="px-3 py-3">
                      <Skeleton className="ml-auto h-5 w-20" />
                    </td>
                  </tr>
                ))
              ) : error ? (
                <tr>
                  <td colSpan={colSpan} className="px-4 py-8 text-center text-error-40">
                    Error loading leaderboard: {error}
                  </td>
                </tr>
              ) : sortedRankings.length ? (
                sortedRankings.map((entry, index) => {
                  const rank = index + 1
                  const noReward = (entry.liveScore?.estimatedReward ?? 0) === 0
                  return (
                    <tr
                      key={`${entry.operator}:${entry.type}`}
                      className={`transition-colors ${
                        noReward
                          ? 'bg-red-500/10 hover:bg-red-500/20'
                          : 'hover:bg-primary-60/50'
                      }`}
                    >
                      <td className="whitespace-nowrap px-3 py-3">
                        <span className="font-space text-sm font-semibold text-white">
                          #{rank}
                        </span>
                      </td>
                      <td className="px-4 py-3">
                        <Link
                          to={`/nodes/${entry.operator}/${entry.type}`}
                          className="block"
                        >
                          {entry.alias && (
                            <span className="text-sm text-primary-30">{entry.alias}</span>
                          )}
                          <span className={`block font-space text-xs tracking-wide ${entry.alias ? 'text-gray-50' : 'text-sm text-primary-30'}`}>
                            {truncateOperator(entry.operator)}
                          </span>
                        </Link>
                      </td>
                      {typeFilter === 'all' && (
                        <td className="whitespace-nowrap px-3 py-3">
                          <div className="flex justify-center">
                            <Badge variant={entry.type === 'lite' ? 'info' : 'warning'}>
                              {entry.type.charAt(0).toUpperCase() + entry.type.slice(1)}
                            </Badge>
                          </div>
                        </td>
                      )}
                      <td className="whitespace-nowrap px-3 py-3 text-center text-sm text-white">
                        {entry.country || '-'}
                      </td>
                      <td className="whitespace-nowrap px-3 py-3 text-right text-sm font-semibold text-primary-30">
                        {(entry.liveScore?.finalScore ?? 0).toFixed(2)}%
                      </td>
                      <td className="whitespace-nowrap px-3 py-3 text-right text-sm text-white">
                        {(entry.liveScore?.rewardPoints ?? 0).toLocaleString(undefined, {
                          maximumFractionDigits: 0
                        })}
                      </td>
                      <td className={`whitespace-nowrap px-3 py-3 text-right text-sm font-medium ${
                        noReward ? 'text-red-500' : 'text-success-40'
                      }`}>
                        {(entry.liveScore?.estimatedReward ?? 0).toLocaleString()} QUS
                      </td>
                    </tr>
                  )
                })
              ) : (
                <tr>
                  <td colSpan={colSpan} className="px-4 py-8 text-center text-gray-50">
                    {search ? 'No results matching your search' : 'No rankings available'}
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        </div>
      </Card>
    </div>
  )
}
