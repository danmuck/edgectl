export interface HealthResponse {
  service: string
  status: string
  uptime?: string
  version?: string
}

export async function fetchHealth(): Promise<HealthResponse> {
  const baseUrl = process.env.EDGECTL_API_URL
  if (!baseUrl) {
    throw new Error("EDGECTL_API_URL is not set in the Next env (.env or .env.local)")
  }

  console.log("fetchHealth baseUrl:", baseUrl, "/health")

  const res = await fetch(
    `${baseUrl}/health`,
    { cache: "no-store" } // always fresh
  )

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

  console.log("sendReboot baseUrl:", baseUrl, "/reboot")

  const res = await fetch(
    `${baseUrl}/reboot`,
    { cache: "no-store" } // always fresh
  )

  if (!res.ok) {
    throw new Error("Reboot check failed")
  }

  return res.json()
}
