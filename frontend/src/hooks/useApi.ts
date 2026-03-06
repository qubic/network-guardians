import { useState, useEffect, useCallback } from 'react'

const API_BASE_URL = import.meta.env.VITE_API_URL || ''

interface UseApiResult<T> {
  data: T | null
  loading: boolean
  error: string | null
  refetch: () => void
}

export function useApi<T>(url: string, pollInterval?: number): UseApiResult<T> {
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const fullUrl = `${API_BASE_URL}${url}`

  const fetchData = useCallback(async () => {
    try {
      const response = await fetch(fullUrl)
      if (!response.ok) {
        const text = await response.text()
        throw new Error(`HTTP ${response.status}: ${text || response.statusText}`)
      }
      const json = await response.json()
      setData(json)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'An error occurred')
    } finally {
      setLoading(false)
    }
  }, [fullUrl])

  useEffect(() => {
    fetchData()

    if (pollInterval && pollInterval > 0) {
      const interval = setInterval(fetchData, pollInterval)
      return () => clearInterval(interval)
    }
  }, [fetchData, pollInterval])

  return { data, loading, error, refetch: fetchData }
}
