export interface HealthResponse {
  service: string
  status: string
  uptime?: string
  version?: string
}

export type HealthTarget = {
  label: string
  apiUrl: string
}

export async function fetchHealth(baseUrl: string): Promise<HealthResponse> {
  if (!baseUrl) {
    throw new Error("API base URL is required for health checks")
  }

  const res = await fetch(buildHealthUrl(baseUrl), { cache: "no-store" })

  if (!res.ok) {
    throw new Error("Health check failed")
  }

  return res.json()
}

export interface RebootResponse {
  status: string
  uptime?: string
  version?: string
}

export async function sendReboot(): Promise<RebootResponse> {
  const baseUrl = process.env.EDGECTL_API_URL
  if (!baseUrl) {
    throw new Error("EDGECTL_API_URL is not set in the Next env (.env or .env.local)")
  }

  const res = await fetch(
    `${baseUrl}/reboot`,
    { cache: "no-store" } // always fresh
  )

  if (!res.ok) {
    throw new Error("Reboot check failed")
  }

  return res.json()
}

export function buildHealthUrl(baseUrl: string) {
  return `${stripTrailingSlash(baseUrl)}/health`
}

export function parseHealthTargets(
  rawTargets: string | undefined,
  fallbackUrl?: string,
  fallbackLabel = "edge-api"
): HealthTarget[] {
  const targets: HealthTarget[] = []
  const raw = rawTargets?.trim()

  if (raw) {
    const entries = raw.split(",").map((entry) => entry.trim()).filter(Boolean)
    for (const entry of entries) {
      const [labelCandidate, urlCandidate] = entry.split("|").map((part) => part.trim())
      if (urlCandidate) {
        targets.push({
          label: labelCandidate || labelFromUrl(urlCandidate) || fallbackLabel,
          apiUrl: urlCandidate,
        })
      } else if (labelCandidate) {
        targets.push({
          label: labelFromUrl(labelCandidate) || labelCandidate,
          apiUrl: labelCandidate,
        })
      }
    }
  }

  if (targets.length === 0 && fallbackUrl) {
    targets.push({ label: fallbackLabel, apiUrl: fallbackUrl })
  }

  return targets
}

function stripTrailingSlash(value: string) {
  return value.endsWith("/") ? value.slice(0, -1) : value
}

function labelFromUrl(value: string) {
  try {
    return new URL(value).host
  } catch {
    return null
  }
}
