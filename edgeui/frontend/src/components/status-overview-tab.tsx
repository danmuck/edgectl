"use client";

import { useEffect, useMemo, useState } from "react";

import { HealthStatusCard } from "@/components/health-status-card";

type SeedInfo = {
	id: string;
	host: string;
	addr: string;
};

type SeedsResponse = {
	seeds: SeedInfo[];
};

type StatusOverviewTabProps = {
	seeds?: SeedInfo[];
	apiBase?: string;
	intervalMs?: number;
};

const DEFAULT_INTERVAL = 15_000;

export function StatusOverviewTab({
	seeds: seedsProp,
	apiBase,
	intervalMs = DEFAULT_INTERVAL,
}: StatusOverviewTabProps) {
	const apiTarget = apiBase ?? process.env.NEXT_PUBLIC_EDGECTL_API_URL;
	const [seeds, setSeeds] = useState<SeedInfo[]>(seedsProp ?? []);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		let cancelled = false;
		let timer: number | null = null;

		if (!apiTarget) {
			setError("NEXT_PUBLIC_EDGECTL_API_URL is not configured");
			return undefined;
		}

		if (seedsProp && seedsProp.length > 0) {
			setSeeds(seedsProp);
			return undefined;
		}

		const load = async () => {
			try {
				const response = await fetch(`${apiTarget}/seeds`, {
					cache: "no-store",
				});
				if (!response.ok) {
					throw new Error("Failed to load seeds");
				}
				const data = (await response.json()) as SeedsResponse;
				if (!cancelled) {
					setSeeds(data.seeds ?? []);
					setError(null);
				}
			} catch (err) {
				if (!cancelled) {
					setError(err instanceof Error ? err.message : "Failed to load seeds");
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
	}, [apiTarget, intervalMs, seedsProp]);

	const targets = useMemo(() => {
		if (!apiTarget) return [];
		return seeds.map((seed) => ({
			label: seed.id,
			apiUrl: `${apiTarget}/seeds/${seed.id}`,
		}));
	}, [apiTarget, seeds]);

	if (error) {
		return (
			<div className="rounded-xl border border-amber-400/40 bg-amber-500/10 p-4 text-sm text-amber-100">
				{error}
			</div>
		);
	}

	return (
		<div className="grid gap-6 md:grid-cols-2">
			{targets.map((target) => (
				<HealthStatusCard
					key={target.apiUrl}
					variant="card"
					targets={[target]}
					intervalMs={5_000}
				/>
			))}
			{targets.length === 0 && (
				<div className="text-sm text-slate-400">No seeds registered yet.</div>
			)}
		</div>
	);
}
