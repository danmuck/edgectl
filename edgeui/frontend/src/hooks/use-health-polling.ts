"use client"

import { useEffect, useState } from "react"

import { fetchHealth, type HealthResponse } from "@/lib/health"

type HealthPollingState = {
  health: HealthResponse | null
  error: string | null
}

type HealthPollingOptions = {
  intervalMs?: number
  immediate?: boolean
}

export function useHealthPolling(
  apiUrl: string | null | undefined,
  options: HealthPollingOptions = {}
): HealthPollingState {
  const { intervalMs = 5_000, immediate = true } = options
  const [health, setHealth] = useState<HealthResponse | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    let timer: number | null = null

    if (!apiUrl) {
      setHealth(null)
      setError("API target is not configured")
      return undefined
    }

    const tick = async () => {
      try {
        const result = await fetchHealth(apiUrl)
        if (!cancelled) {
          setHealth(result)
          setError(null)
        }
      } catch (err) {
        if (!cancelled) {
          const message =
            err instanceof Error ? err.message : "Health check unavailable"
          setError(message)
        }
      }
    }

    if (immediate) {
      void tick()
    }

    timer = window.setInterval(tick, intervalMs)

    return () => {
      cancelled = true
      if (timer) {
        window.clearInterval(timer)
      }
    }
  }, [apiUrl, intervalMs, immediate])

  return { health, error }
}
