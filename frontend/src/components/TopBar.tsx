import { useState, useEffect } from 'react'
import { useApi } from '../hooks/useApi'
import { Stats } from '../types'

function formatPrice(price: number | undefined): string {
  if (!price) return '-'
  if (price < 0.0001) {
    return `$${price.toFixed(8)}`
  }
  if (price < 0.01) {
    return `$${price.toFixed(6)}`
  }
  return `$${price.toFixed(4)}`
}

export default function TopBar() {
  const { data: stats } = useApi<Stats>('/api/v1/stats', 30000)
  const [price, setPrice] = useState<number | undefined>()
  const [priceLoading, setPriceLoading] = useState(true)

  // Fetch QUBIC price from CoinGecko
  useEffect(() => {
    const fetchPrice = async () => {
      try {
        const res = await fetch('https://api.coingecko.com/api/v3/simple/price?ids=qubic-network&vs_currencies=usd')
        const data = await res.json()
        setPrice(data['qubic-network']?.usd)
      } catch {
        // Silently fail
      } finally {
        setPriceLoading(false)
      }
    }
    fetchPrice()
    const interval = setInterval(fetchPrice, 60000) // Update every minute
    return () => clearInterval(interval)
  }, [])

  return (
    <section className="sticky top-0 z-50 border-b border-primary-60 bg-primary-80">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-2.5">
        <p className="flex items-center gap-1.5 text-12 text-gray-50">
          QUBIC Price:{' '}
          {priceLoading ? (
            <span className="inline-block h-3.5 w-16 animate-pulse rounded bg-primary-60" />
          ) : (
            <span className="font-medium text-primary-30">{formatPrice(price)}</span>
          )}
        </p>
        <div className="flex items-center gap-6 text-12 text-gray-50">
          <span>Epoch: <span className="font-medium text-white">{stats?.reference?.epoch ?? '-'}</span></span>
          <span>Tick: <span className="font-medium text-white">{stats?.reference?.tick?.toLocaleString() ?? '-'}</span></span>
        </div>
      </div>
    </section>
  )
}
