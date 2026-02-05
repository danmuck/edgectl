"use client";

import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

type SeedInfo = {
	id: string;
	host: string;
	addr: string;
	services?: string[];
};

type SeedsResponse = {
	seeds: SeedInfo[];
};

type ServiceInfo = {
	name: string;
	actions: string[];
};

type ServicesResponse = {
	services: ServiceInfo[];
};

type EndpointStatus = {
	label: string;
	url: string;
	ok: boolean;
	message?: string;
};

const DEFAULT_INTERVAL = 10_000;

type StatusEndpointsTabProps = {
	intervalMs?: number;
	seedId?: string;
	apiBase?: string;
};

export function StatusEndpointsTab({
	intervalMs = DEFAULT_INTERVAL,
	seedId,
	apiBase: apiBaseProp,
}: StatusEndpointsTabProps) {
	const apiBase = apiBaseProp ?? process.env.NEXT_PUBLIC_EDGECTL_API_URL;
	const [endpoints, setEndpoints] = useState<EndpointStatus[]>([]);
	const [seedServices, setSeedServices] = useState<Record<string, ServiceInfo[]>>({});
	const [seeds, setSeeds] = useState<SeedInfo[]>([]);
	const [error, setError] = useState<string | null>(null);
	const [updatedAt, setUpdatedAt] = useState<Date | null>(null);

	const baseEndpoints = useMemo(() => {
		if (!apiBase || seedId) return [];
		return [
			{ label: "ghost /health", url: `${apiBase}/health` },
			{ label: "ghost /ready", url: `${apiBase}/ready` },
			{ label: "ghost /seeds", url: `${apiBase}/seeds` },
			{ label: "ghost /metrics", url: `${apiBase}/metrics` },
		];
	}, [apiBase, seedId]);

	useEffect(() => {
		let cancelled = false;
		let timer: number | null = null;

		if (!apiBase) {
			setError("NEXT_PUBLIC_EDGECTL_API_URL is not configured");
			return undefined;
		}

		const load = async () => {
			try {
				const seedsResponse = await fetchJSON<SeedsResponse>(`${apiBase}/seeds`);
				const seedList = seedsResponse.seeds ?? [];
				const scopedSeeds = seedId
					? seedList.filter((seed) => seed.id === seedId)
					: seedList;

				if (seedId && scopedSeeds.length === 0) {
					throw new Error(`Seed not found: ${seedId}`);
				}

				const seedEndpoints = scopedSeeds.flatMap((seed) => [
					{
						label: `${seed.id} /health`,
						url: `${apiBase}/seeds/${seed.id}/health`,
					},
					{
						label: `${seed.id} /ready`,
						url: `${apiBase}/seeds/${seed.id}/ready`,
					},
					{
						label: `${seed.id} /services`,
						url: `${apiBase}/seeds/${seed.id}/services`,
					},
					{
						label: `${seed.id} /metrics`,
						url: `${apiBase}/seeds/${seed.id}/metrics`,
					},
				]);

				const endpointChecks = [...baseEndpoints, ...seedEndpoints];
				const results = await Promise.all(
					endpointChecks.map((endpoint) => checkEndpoint(endpoint)),
				);

				const servicesEntries = await Promise.all(
					scopedSeeds.map(async (seed) => {
						try {
							const response = await fetchJSON<ServicesResponse>(
								`${apiBase}/seeds/${seed.id}/services`,
							);
							return [seed.id, response.services ?? []] as const;
						} catch {
							return [seed.id, []] as const;
						}
					}),
				);

				if (!cancelled) {
					setEndpoints(results);
					setSeeds(scopedSeeds);
					setSeedServices(Object.fromEntries(servicesEntries));
					setError(null);
					setUpdatedAt(new Date());
				}
			} catch (err) {
				if (!cancelled) {
					const message =
						err instanceof Error ? err.message : "Failed to load endpoints";
					setError(message);
				}
			}
		};

		void load();
		timer = window.setInterval(load, intervalMs);

		return () => {
			cancelled = true;
			if (timer) {
				window.clearInterval(timer);
			}
		};
	}, [apiBase, baseEndpoints, intervalMs, seedId]);

	return (
		<div className="flex flex-col gap-6">
			<Card className="border-white/10 bg-gradient-to-br from-slate-950 via-slate-900 to-slate-950 text-slate-100">
				<CardHeader className="border-b border-white/10 pb-4">
					<CardTitle className="text-lg">Endpoint Status</CardTitle>
					{updatedAt && (
						<p className="text-xs text-slate-400">
							Last checked {updatedAt.toLocaleTimeString()}
						</p>
					)}
				</CardHeader>
				<CardContent className="space-y-3">
					{error && (
						<div className="rounded-lg border border-amber-400/30 bg-amber-500/10 px-3 py-2 text-xs text-amber-100">
							{error}
						</div>
					)}
					<ul className="space-y-2 text-sm">
						{endpoints.map((endpoint) => (
							<li
								key={endpoint.url}
								className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-white/5 bg-white/5 px-3 py-2"
							>
								<div className="flex flex-col">
									<span className="text-slate-200">{endpoint.label}</span>
									<span className="text-xs text-slate-400">
										{endpoint.url}
									</span>
								</div>
								<Badge
									variant={endpoint.ok ? "default" : "destructive"}
									className={cn(
										"shrink-0",
										endpoint.ok
											? "bg-emerald-500 text-emerald-950"
											: "bg-rose-500 text-rose-50",
									)}
								>
									{endpoint.ok ? "OK" : "Down"}
								</Badge>
							</li>
						))}
						{endpoints.length === 0 && (
							<li className="text-xs text-slate-400">
								No endpoints loaded yet.
							</li>
						)}
					</ul>
				</CardContent>
			</Card>

			<Card className="border-white/10 bg-gradient-to-br from-slate-950 via-slate-900 to-slate-950 text-slate-100">
				<CardHeader className="border-b border-white/10 pb-4">
					<CardTitle className="text-lg">Command Index</CardTitle>
					<p className="text-xs text-slate-400">
						{seedId ? "Commands available for this seed" : "Commands available per seed"}
					</p>
				</CardHeader>
				<CardContent className="space-y-4">
					{seeds.map((seed) => {
						const services = seedServices[seed.id] ?? [];
						return (
							<div
								key={seed.id}
								className="rounded-lg border border-white/5 bg-white/5 px-4 py-3"
							>
								<div className="mb-2 flex items-center justify-between">
									<div className="text-sm text-slate-100">{seed.id}</div>
									<span className="text-xs text-slate-400">
										{seed.host || "local"} · {seed.addr}
									</span>
								</div>
								{services.length === 0 ? (
									<span className="text-xs text-slate-400">
										No registered services.
									</span>
								) : (
									<div className="flex flex-col gap-2 text-xs text-slate-200">
										{services.map((service) => (
											<div key={service.name}>
												<span className="font-semibold text-slate-100">
													{service.name}
												</span>
												<div className="mt-1 flex flex-wrap gap-2">
													{service.actions.map((action) => (
														<span
															key={`${service.name}-${action}`}
															className="rounded-full border border-white/10 bg-white/10 px-2 py-0.5"
														>
															{action}
														</span>
													))}
												</div>
											</div>
										))}
									</div>
								)}
							</div>
						);
					})}
					{seeds.length === 0 && (
						<div className="text-xs text-slate-400">
							No seeds registered yet.
						</div>
					)}
				</CardContent>
			</Card>
		</div>
	);
}

async function fetchJSON<T>(url: string): Promise<T> {
	const response = await fetch(url, { cache: "no-store" });
	if (!response.ok) {
		throw new Error(`Request failed: ${url}`);
	}
	return response.json();
}

async function checkEndpoint(endpoint: { label: string; url: string }): Promise<EndpointStatus> {
	try {
		const response = await fetch(endpoint.url, { cache: "no-store" });
		return {
			label: endpoint.label,
			url: endpoint.url,
			ok: response.ok,
		};
	} catch (err) {
		return {
			label: endpoint.label,
			url: endpoint.url,
			ok: false,
			message: err instanceof Error ? err.message : "request failed",
		};
	}
}
