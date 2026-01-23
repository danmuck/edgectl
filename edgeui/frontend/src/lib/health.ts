export interface HealthResponse {
  status: string
  uptime?: string
  version?: string
}

export async function fetchHealth(): Promise<HealthResponse> {
  const baseUrl = process.env.EDGECTL_API_URL
  if (!baseUrl) {
    throw new Error("EDGECTL_API_URL is not set in the Next env (.env or .env.local)")
  }

  console.log("fetchHealth baseUrl:", baseUrl)

  const res = await fetch(
    `${baseUrl}/health`,
    { cache: "no-store" } // always fresh
  )

  if (!res.ok) {
    throw new Error("Health check failed")
  }

  return res.json()
}
