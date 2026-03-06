import { useState, useEffect, useRef, useMemo, lazy, Suspense, Component, ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useApi } from '../hooks/useApi'
import { Stats, Health, Node } from '../types'
import Card from '../components/Card'
import StatsCard from '../components/StatsCard'
import Skeleton from '../components/Skeleton'
import type { GlobeMethods } from 'react-globe.gl'

const Globe = lazy(() => import('react-globe.gl'))

// Error handling for Globe component
interface ErrorBoundaryProps {
  children: ReactNode
  fallback: ReactNode
}

interface ErrorBoundaryState {
  hasError: boolean
}

class GlobeErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props)
    this.state = { hasError: false }
  }

  static getDerivedStateFromError(): ErrorBoundaryState {
    return { hasError: true }
  }

  render() {
    if (this.state.hasError) {
      return this.props.fallback
    }
    return this.props.children
  }
}

interface GlobePoint {
  lat: number
  lng: number
  size: number
  color: string
  operator: string
  type: string
}

interface GlobeArc {
  startLat: number
  startLng: number
  endLat: number
  endLng: number
  color: string
}


function formatNumber(num: number): string {
  return new Intl.NumberFormat().format(num)
}

function formatTimeRemaining(seconds: number): string {
  const days = Math.floor(seconds / 86400)
  const hours = Math.floor((seconds % 86400) / 3600)
  const mins = Math.floor((seconds % 3600) / 60)

  if (days > 0) return `${days}d ${hours}h`
  if (hours > 0) return `${hours}h ${mins}m`
  return `${mins}m`
}

const NodesIcon = () => (
  <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 12h14M5 12a2 2 0 01-2-2V6a2 2 0 012-2h14a2 2 0 012 2v4a2 2 0 01-2 2M5 12a2 2 0 00-2 2v4a2 2 0 002 2h14a2 2 0 002-2v-4a2 2 0 00-2-2m-2-4h.01M17 16h.01" />
  </svg>
)

const RewardIcon = () => (
  <svg className="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor">
    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 8c-1.657 0-3 .895-3 2s1.343 2 3 2 3 .895 3 2-1.343 2-3 2m0-8c1.11 0 2.08.402 2.599 1M12 8V7m0 1v8m0 0v1m0-1c-1.11 0-2.08-.402-2.599-1M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
  </svg>
)

export default function Dashboard() {
  const { data: stats, loading: statsLoading, error: statsError } = useApi<Stats>('/api/v1/stats', 30000)
  const { data: health, loading: healthLoading, error: healthError } = useApi<Health>('/health', 10000)
  const { data: nodes } = useApi<Node[]>('/api/v1/nodes', 30000)
  const globeRef = useRef<GlobeMethods | undefined>(undefined)
  const containerRef = useRef<HTMLDivElement>(null)
  const [globeReady, setGlobeReady] = useState(false)
  const [showGlobe, setShowGlobe] = useState(false)
  const [dimensions, setDimensions] = useState({ width: 800, height: 600 })
  const [webglSupported, setWebglSupported] = useState<boolean | null>(null)

  const loading = statsLoading || healthLoading
  const error = statsError || healthError

  useEffect(() => {
    try {
      const canvas = document.createElement('canvas')
      const gl = canvas.getContext('webgl') || canvas.getContext('experimental-webgl')
      setWebglSupported(!!gl)
    } catch {
      setWebglSupported(false)
    }
  }, [])

  // loading until main content is ready
  useEffect(() => {
    if (!loading && stats) {
      const timer = setTimeout(() => setShowGlobe(true), 100)
      return () => clearTimeout(timer)
    }
  }, [loading, stats])

  // Track container size
  useEffect(() => {
    const container = containerRef.current
    if (!container) return

    const updateDimensions = () => {
      setDimensions({
        width: container.clientWidth,
        height: container.clientHeight
      })
    }

    updateDimensions()
    const resizeObserver = new ResizeObserver(updateDimensions)
    resizeObserver.observe(container)

    return () => resizeObserver.disconnect()
  }, [])

  const liteColor = '#61F0FE'
  const bobColor = '#F59E0B'

  // Convert nodes to globe points (exclude flagged nodes)
  const globePoints = useMemo<GlobePoint[]>(() => {
    if (!nodes) return []
    return nodes
      .filter(n => n.latitude != null && n.longitude != null && !n.flagged)
      .map(n => ({
        lat: n.latitude!,
        lng: n.longitude!,
        size: 0.3,
        color: n.type === 'lite' ? liteColor : bobColor,
        operator: n.operator,
        type: n.type
      }))
  }, [nodes])

  // pulsing rings at each node
  const ringsData = useMemo(() => {
    return globePoints.map(p => ({
      lat: p.lat,
      lng: p.lng,
      nodeType: p.type,
      color: p.type === 'bob' ? bobColor : liteColor
    }))
  }, [globePoints, liteColor, bobColor])

  // Generate random arcs between nodes
  const [arcs, setArcs] = useState<GlobeArc[]>([])

  useEffect(() => {
    if (globePoints.length < 2) return

    // Calculate angular distance between two points (in degrees)
    const angularDistance = (lat1: number, lng1: number, lat2: number, lng2: number) => {
      const dLat = Math.abs(lat1 - lat2)
      const dLng = Math.abs(lng1 - lng2)
      return Math.sqrt(dLat * dLat + dLng * dLng)
    }

    const generateArcs = () => {
      const numArcs = Math.max(4, Math.min(Math.floor(globePoints.length / 2), 8))
      const newArcs: GlobeArc[] = []
      const usedPairs = new Set<string>()

      for (let i = 0; i < numArcs; i++) {
        let attempts = 0
        while (attempts < 30) {
          const from = globePoints[Math.floor(Math.random() * globePoints.length)]
          const to = globePoints[Math.floor(Math.random() * globePoints.length)]

          if (from.operator === to.operator) {
            attempts++
            continue
          }

          // Skip if nodes are too far apart (would clip through globe)
          const distance = angularDistance(from.lat, from.lng, to.lat, to.lng)
          if (distance > 120) {
            attempts++
            continue
          }

          const pairKey = [from.operator, to.operator].sort().join('-')
          if (usedPairs.has(pairKey)) {
            attempts++
            continue
          }

          usedPairs.add(pairKey)
          newArcs.push({
            startLat: from.lat,
            startLng: from.lng,
            endLat: to.lat,
            endLng: to.lng,
            color: from.color
          })
          break
        }
      }
      return newArcs
    }

    setArcs(generateArcs())
    const interval = setInterval(() => setArcs(generateArcs()), 3000)
    return () => clearInterval(interval)
  }, [globePoints])

  // Configure globe when ready
  useEffect(() => {
    if (globeReady && globeRef.current) {
      globeRef.current.controls().autoRotate = true
      globeRef.current.controls().autoRotateSpeed = 1.2
      globeRef.current.controls().enableZoom = false
      globeRef.current.pointOfView({ altitude: 2.5 })
    }
  }, [globeReady])

  return (
    <div className="space-y-8">
      {/* Page Header */}
      <div className="flex items-center justify-between">
        <div>
          <h1 className="font-space text-24 font-bold text-white">Dashboard</h1>
          <p className="mt-1 text-14 text-gray-50">
            Overview of the Qubic Guardians network
          </p>
        </div>
        {/* Health Status */}
        {health && (
          <div className="flex items-center gap-2">
            <span
              className={`h-2 w-2 rounded-full ${
                health.healthy ? 'bg-success-40' : 'bg-error-40'
              }`}
            />
            <span className="text-13 text-gray-50">
              {health.healthy ? 'Service Healthy' : 'Service Unhealthy'}
            </span>
          </div>
        )}
      </div>

      {/* Stats Grid */}
      {loading ? (
        <div className="space-y-4">
          <Skeleton className="h-24" />
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <Skeleton key={i} className="h-24" />
            ))}
          </div>
        </div>
      ) : stats ? (
        <>
          {/* Epoch Progress - At Top */}
          <Card className="p-5">
            <div className="flex items-center justify-between">
              <h2 className="font-space text-16 font-semibold text-white">
                Epoch {stats.reference.epoch} Progress
              </h2>
              <span className="text-13 text-gray-50">
                {formatTimeRemaining(stats.epochProgress.time_remaining_seconds)} remaining
              </span>
            </div>
            <div className="mt-3">
              <div className="h-2.5 overflow-hidden rounded-full bg-primary-60">
                <div
                  className="h-full rounded-full bg-primary-30 transition-all duration-500"
                  style={{ width: `${stats.epochProgress.progress_percent}%` }}
                />
              </div>
              <div className="mt-2 text-right text-13 text-white">
                {stats.epochProgress.progress_percent.toFixed(1)}%
              </div>
            </div>
          </Card>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <StatsCard
              label="Total Nodes"
              value={formatNumber(stats.totalNodes)}
              icon={<NodesIcon />}
            />
            <StatsCard
              label="Lite Nodes"
              value={formatNumber(stats.liteNodes)}
              icon={<NodesIcon />}
            />
            <StatsCard
              label="Bob Nodes"
              value={formatNumber(stats.bobNodes)}
              icon={<NodesIcon />}
            />
          </div>

          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <StatsCard
              label="Total Reward Pool"
              value={formatNumber(stats.epochRewards.totalPool)}
              icon={<RewardIcon />}
              subValue="QUBIC"
            />
            <StatsCard
              label="Lite Pool"
              value={formatNumber(stats.epochRewards.litePool)}
              icon={<RewardIcon />}
              subValue={`${((stats.epochRewards.litePool / stats.epochRewards.totalPool) * 100).toFixed(0)}%`}
            />
            <StatsCard
              label="Bob Pool"
              value={formatNumber(stats.epochRewards.bobPool)}
              icon={<RewardIcon />}
              subValue={`${((stats.epochRewards.bobPool / stats.epochRewards.totalPool) * 100).toFixed(0)}%`}
            />
          </div>

          {/* 3D Globe Preview */}
          <Card className="overflow-hidden">
            <div className="flex items-center justify-between border-b border-primary-60 px-5 py-3">
              <h2 className="font-space text-16 font-semibold text-white">
                Node Distribution
              </h2>
              <Link to="/map" className="text-13 text-primary-30 hover:underline">
                View Full Map →
              </Link>
            </div>
            <div
              ref={containerRef}
              className="relative overflow-hidden flex items-center justify-center bg-black"
              style={{ height: '600px' }}
            >
              {webglSupported === false ? (
                <div className="flex flex-col items-center justify-center gap-4 p-8 text-center">
                  <svg className="h-16 w-16 text-primary-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                  </svg>
                  <div>
                    <p className="text-white font-medium">3D Globe unavailable</p>
                    <p className="mt-1 text-sm text-gray-50">Your browser doesn't support WebGL</p>
                  </div>
                  <Link
                    to="/map"
                    className="mt-2 rounded-lg bg-primary-50 px-4 py-2 text-sm font-medium text-white hover:bg-primary-40 transition-colors"
                  >
                    View 2D Map Instead
                  </Link>
                </div>
              ) : showGlobe ? (
                <GlobeErrorBoundary
                  fallback={
                    <div className="flex flex-col items-center justify-center gap-4 p-8 text-center">
                      <svg className="h-16 w-16 text-primary-40" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={1.5} d="M3.055 11H5a2 2 0 012 2v1a2 2 0 002 2 2 2 0 012 2v2.945M8 3.935V5.5A2.5 2.5 0 0010.5 8h.5a2 2 0 012 2 2 2 0 104 0 2 2 0 012-2h1.064M15 20.488V18a2 2 0 012-2h3.064M21 12a9 9 0 11-18 0 9 9 0 0118 0z" />
                      </svg>
                      <div>
                        <p className="text-white font-medium">3D Globe unavailable</p>
                        <p className="mt-1 text-sm text-gray-50">Your browser may not support WebGL</p>
                      </div>
                      <Link
                        to="/map"
                        className="mt-2 rounded-lg bg-primary-50 px-4 py-2 text-sm font-medium text-white hover:bg-primary-40 transition-colors"
                      >
                        View 2D Map Instead
                      </Link>
                    </div>
                  }
                >
                  <Suspense fallback={
                    <div className="flex flex-col items-center justify-center gap-4">
                      <div className="h-12 w-12 animate-spin rounded-full border-4 border-primary-60 border-t-primary-30" />
                      <span className="text-sm text-gray-50">Loading globe...</span>
                    </div>
                  }>
                    <div style={{
                      position: 'absolute',
                      left: '50%',
                      top: '50%',
                      transform: 'translate(-50%, -50%)'
                    }}>
                      <Globe
                        ref={globeRef}
                        width={dimensions.width * 1.5}
                        height={dimensions.height * 1.5}
                        backgroundColor="rgba(0,0,0,0)"
                        backgroundImageUrl="//unpkg.com/three-globe/example/img/night-sky.png"
                        globeImageUrl="//unpkg.com/three-globe/example/img/earth-night.jpg"
                        bumpImageUrl="//unpkg.com/three-globe/example/img/earth-topology.png"
                        animateIn={false}
                        atmosphereColor="#61F0FE"
                        atmosphereAltitude={0.08}
                        pointsData={globePoints}
                        pointLat="lat"
                        pointLng="lng"
                        pointColor="color"
                        pointAltitude={0.01}
                        pointRadius="size"
                        ringsData={ringsData}
                        ringLat="lat"
                        ringLng="lng"
                        ringColor={((d: any) => d.nodeType === 'bob' ? bobColor : liteColor) as any}
                        ringMaxRadius={2}
                        ringPropagationSpeed={1}
                        ringRepeatPeriod={1500}
                        arcsData={arcs}
                        arcStartLat="startLat"
                        arcStartLng="startLng"
                        arcEndLat="endLat"
                        arcEndLng="endLng"
                        arcColor="color"
                        arcDashLength={0.15}
                        arcDashGap={0.05}
                        arcDashAnimateTime={2500}
                        arcStroke={0.3}
                        arcAltitudeAutoScale={0.25}
                        arcsTransitionDuration={0}
                        onGlobeReady={() => setGlobeReady(true)}
                      />
                    </div>
                  </Suspense>
                </GlobeErrorBoundary>
              ) : (
                <div className="flex flex-col items-center justify-center gap-4">
                  <div className="h-12 w-12 animate-spin rounded-full border-4 border-primary-60 border-t-primary-30" />
                  <span className="text-sm text-gray-50">Loading globe...</span>
                </div>
              )}
            </div>
            <div className="flex gap-4 border-t border-primary-60 px-5 py-2 text-xs text-gray-50">
              <div className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full" style={{ backgroundColor: liteColor }} />
                <span>Lite</span>
              </div>
              <div className="flex items-center gap-1.5">
                <span className="h-2 w-2 rounded-full" style={{ backgroundColor: bobColor }} />
                <span>Bob</span>
              </div>
              <span className="ml-auto">{globePoints.length} nodes mapped</span>
            </div>
          </Card>
        </>
      ) : (
        <Card className="p-8 text-center">
          <p className="text-gray-50">Unable to load stats. Is the API running?</p>
          {error && <p className="mt-2 text-sm text-error-40">{error}</p>}
        </Card>
      )}
    </div>
  )
}
