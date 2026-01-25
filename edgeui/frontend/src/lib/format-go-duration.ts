const GO_DURATION_RE = /(-?\d+(?:\.\d+)?)(ns|us|µs|ms|s|m|h)/g

type FormatOptions = {
  maxUnits?: number
}

export function formatGoDuration(input: string, options: FormatOptions = {}) {
  const trimmed = input.trim()
  if (!trimmed) return input

  let sign = 1
  let value = trimmed
  if (value.startsWith("-")) {
    sign = -1
    value = value.slice(1)
  }

  let totalMs = 0
  let matched = false

  for (const match of value.matchAll(GO_DURATION_RE)) {
    matched = true
    const amount = Number.parseFloat(match[1])
    const unit = match[2]
    const unitMs =
      unit === "h"
        ? 3_600_000
        : unit === "m"
          ? 60_000
          : unit === "s"
            ? 1_000
            : unit === "ms"
              ? 1
              : unit === "us" || unit === "µs"
                ? 0.001
                : 0.000001
    totalMs += amount * unitMs
  }

  if (!matched) return input

  return formatMilliseconds(totalMs * sign, options.maxUnits)
}

function formatMilliseconds(totalMs: number, maxUnits = 3) {
  const sign = totalMs < 0 ? "-" : ""
  let ms = Math.abs(totalMs)

  if (ms < 1) return `${sign}0s`

  if (ms < 1000) {
    return `${sign}${Math.round(ms)}ms`
  }

  if (ms < 60_000) {
    const seconds = ms / 1000
    const formatted =
      Number.isInteger(seconds) ? seconds.toFixed(0) : seconds.toFixed(1)
    return `${sign}${formatted}s`
  }

  const totalSeconds = Math.floor(ms / 1000)
  const days = Math.floor(totalSeconds / 86_400)
  const hours = Math.floor((totalSeconds % 86_400) / 3_600)
  const minutes = Math.floor((totalSeconds % 3_600) / 60)
  const seconds = totalSeconds % 60

  const parts: string[] = []
  if (days) parts.push(`${days}d`)
  if (hours) parts.push(`${hours}h`)
  if (minutes) parts.push(`${minutes}m`)
  if (seconds || parts.length === 0) parts.push(`${seconds}s`)

  return `${sign}${parts.slice(0, maxUnits).join(" ")}`
}
