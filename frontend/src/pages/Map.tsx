import { useState, useMemo } from 'react'
import { Link } from 'react-router-dom'
import {
  ComposableMap,
  Geographies,
  Geography,
  Marker,
  ZoomableGroup
} from 'react-simple-maps'
import { useApi } from '../hooks/useApi'
import { Node } from '../types'

const GEO_URL = 'https://cdn.jsdelivr.net/npm/world-atlas@2/countries-110m.json'

function truncateOperator(operator: string): string {
  if (operator.length <= 16) return operator
  return `${operator.slice(0, 8)}...${operator.slice(-8)}`
}

export default function Map() {
  const { data: nodes, loading, error } = useApi<Node[]>('/api/v1/nodes', 30000)
  const [filter, setFilter] = useState<'all' | 'lite' | 'bob'>('all')
  const [hoveredNode, setHoveredNode] = useState<Node | null>(null)
  const [selectedNode, setSelectedNode] = useState<Node | null>(null)

  const filteredNodes = useMemo(() => {
    return nodes?.filter((node) => {
      if (node.flagged) return false
      if (filter !== 'all' && node.type !== filter) return false
      return node.latitude != null && node.longitude != null &&
             !isNaN(Number(node.latitude)) && !isNaN(Number(node.longitude))
    }) || []
  }, [nodes, filter])

  const liteColor = '#61F0FE'
  const bobColor = '#F59E0B'
  const activeNode = hoveredNode || selectedNode

  // Calculate country distribution
  const countryDistribution = useMemo(() => {
    if (!filteredNodes.length) return []

    const countryCount: Record<string, number> = {}
    filteredNodes.forEach(node => {
      const country = node.country || 'Unknown'
      countryCount[country] = (countryCount[country] || 0) + 1
    })

    return Object.entries(countryCount)
      .sort((a, b) => b[1] - a[1])
      .slice(0, 8)
  }, [filteredNodes])

  return (
    <div className="fixed inset-0 top-[88px] bg-[#0a1015]">
      {/* CSS for ripple animation */}
      <style>{`
        @keyframes ripple {
          0% { r: 1; opacity: 0.6; }
          100% { r: 6; opacity: 0; }
        }
        .node-ripple {
          animation: ripple 2s ease-out infinite;
        }
      `}</style>

      {/* Filter Controls */}
      <div className="absolute left-4 top-4 z-10 flex items-center gap-2">
        {(['all', 'lite', 'bob'] as const).map((type) => (
          <button
            key={type}
            onClick={() => setFilter(type)}
            className={`rounded-lg px-4 py-2 text-sm font-medium capitalize transition-colors ${
              filter === type
                ? 'bg-primary-30 text-primary-80'
                : 'bg-primary-70/90 text-gray-50 hover:text-white backdrop-blur'
            }`}
          >
            {type}
          </button>
        ))}
        <span className="ml-2 rounded-lg bg-primary-70/90 px-3 py-2 text-sm text-gray-50 backdrop-blur">
          {filteredNodes.length} nodes
        </span>
      </div>

      {/* Legend */}
      <div className="absolute bottom-4 left-4 z-10 flex gap-4 rounded-lg bg-primary-70/90 px-4 py-2 text-sm text-gray-50 backdrop-blur">
        <div className="flex items-center gap-2">
          <div className="h-3 w-3 rounded-full" style={{ backgroundColor: liteColor }} />
          <span>Lite</span>
        </div>
        <div className="flex items-center gap-2">
          <div className="h-3 w-3 rounded-full" style={{ backgroundColor: bobColor }} />
          <span>Bob</span>
        </div>
      </div>

      {/* Country Distribution */}
      {countryDistribution.length > 0 && (
        <div className="absolute right-4 top-4 z-10 w-64 rounded-lg border border-primary-60 bg-primary-70/95 p-4 backdrop-blur">
          <h3 className="mb-3 font-space text-sm font-semibold text-white">
            Top Countries
          </h3>
          <div className="space-y-2">
            {countryDistribution.map(([country, count]) => {
              const percentage = (count / filteredNodes.length) * 100
              return (
                <div key={country} className="space-y-1">
                  <div className="flex justify-between text-xs">
                    <span className="text-gray-50">{country}</span>
                    <span className="text-white">{count}</span>
                  </div>
                  <div className="h-1.5 overflow-hidden rounded-full bg-primary-60">
                    <div
                      className="h-full rounded-full bg-primary-30 transition-all"
                      style={{ width: `${percentage}%` }}
                    />
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}

      {/* Node Info Card */}
      {activeNode && (
        <div className="absolute bottom-4 right-4 z-10 w-72 rounded-lg border border-primary-60 bg-primary-70/95 p-4 backdrop-blur">
          <div className="space-y-2">
            <p className="font-space text-sm font-semibold text-white">
              {activeNode.alias || truncateOperator(activeNode.operator)}
            </p>
            <div className="space-y-1 text-sm">
              <p className="text-gray-50">
                Type: <span className="capitalize text-white">{activeNode.type}</span>
              </p>
              <p className="text-gray-50">
                Location: <span className="text-white">{activeNode.country || 'Unknown'}</span>
              </p>
              <p className="text-gray-50">
                Score: <span className="text-primary-30 font-semibold">{(activeNode.liveScore?.finalScore ?? 0).toFixed(1)}%</span>
              </p>
            </div>
            <Link
              to={`/nodes/${activeNode.operator}/${activeNode.type}`}
              className="mt-2 inline-block text-sm text-primary-30 hover:underline"
            >
              View Details →
            </Link>
          </div>
        </div>
      )}

      {/* Map */}
      <div className="h-full w-full">
        {loading ? (
          <div className="flex h-full items-center justify-center">
            <p className="text-gray-50">Loading map data...</p>
          </div>
        ) : error ? (
          <div className="flex h-full items-center justify-center">
            <p className="text-error-40">Error loading nodes: {error}</p>
          </div>
        ) : (
          <ComposableMap
            projection="geoMercator"
            projectionConfig={{ scale: 130, center: [0, 20] }}
            style={{ width: '100%', height: '100%', background: '#0a1015' }}
          >
            <ZoomableGroup center={[0, 20]} zoom={1}>
              <Geographies geography={GEO_URL}>
                {({ geographies }) =>
                  geographies.map((geo) => (
                    <Geography
                      key={geo.rsmKey}
                      geography={geo}
                      fill="#1a252f"
                      stroke="#2a3a48"
                      strokeWidth={0.5}
                      style={{
                        default: { outline: 'none' },
                        hover: { outline: 'none', fill: '#243040' },
                        pressed: { outline: 'none' }
                      }}
                    />
                  ))
                }
              </Geographies>

              {/* Node markers with ripple */}
              {filteredNodes.map((node) => {
                const isActive = activeNode?.operator === node.operator
                const color = node.type === 'lite' ? liteColor : bobColor

                return (
                  <Marker
                    key={node.operator}
                    coordinates={[node.longitude!, node.latitude!]}
                    onMouseEnter={() => setHoveredNode(node)}
                    onMouseLeave={() => setHoveredNode(null)}
                    onClick={() => setSelectedNode(selectedNode?.operator === node.operator ? null : node)}
                  >
                    {/* Expanding ripple */}
                    <circle
                      r={1}
                      fill="none"
                      stroke={color}
                      strokeWidth={0.3}
                      className="node-ripple"
                    />
                    {/* Main dot */}
                    <circle
                      r={isActive ? 1.5 : 1}
                      fill={color}
                      style={{ cursor: 'pointer' }}
                    />
                  </Marker>
                )
              })}
            </ZoomableGroup>
          </ComposableMap>
        )}
      </div>
    </div>
  )
}
