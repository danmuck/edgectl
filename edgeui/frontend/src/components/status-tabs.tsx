"use client";

import { useEffect, useMemo, useState } from "react";

import { PageTabs } from "@/components/page-tabs";
import { StatusEndpointsTab } from "@/components/status-endpoints-tab";
import { StatusOverviewTab } from "@/components/status-overview-tab";

type SeedInfo = {
	id: string;
	host: string;
	addr: string;
};

type SeedsResponse = {
	seeds: SeedInfo[];
};

export function StatusTabs() {
	const apiBase = process.env.NEXT_PUBLIC_EDGECTL_API_URL;
	const [seeds, setSeeds] = useState<SeedInfo[]>([]);
	const [error, setError] = useState<string | null>(null);

	useEffect(() => {
		let cancelled = false;

		if (!apiBase) {
			setError("NEXT_PUBLIC_EDGECTL_API_URL is not configured");
			return undefined;
		}

		const load = async () => {
			try {
				const response = await fetch(`${apiBase}/seeds`, { cache: "no-store" });
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
		return () => {
			cancelled = true;
		};
	}, [apiBase]);

	const tabs = useMemo(() => {
		const baseTabs = [
			{
				value: "overview",
				label: "Overview",
				content: <StatusOverviewTab apiBase={apiBase} seeds={seeds} />,
			},
		];

		const seedTabs = seeds.map((seed) => ({
			value: `seed-${seed.id}`,
			label: seed.id,
			content: (
				<StatusEndpointsTab apiBase={apiBase} seedId={seed.id} intervalMs={10_000} />
			),
		}));

		return [...baseTabs, ...seedTabs];
	}, [apiBase, seeds]);

	if (error) {
		return (
			<div className="rounded-xl border border-amber-400/40 bg-amber-500/10 p-4 text-sm text-amber-100">
				{error}
			</div>
		);
	}

	return <PageTabs tabs={tabs} lazy />;
}
