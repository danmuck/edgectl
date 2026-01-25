import { HealthStatusCard } from "@/components/health-status-card";
import { parseHealthTargets } from "@/lib/health";

export default function HomePage() {
	const apiTarget =
		process.env.EDGECTL_API_URL ?? process.env.NEXT_PUBLIC_EDGECTL_API_URL;
	const apiTargetList =
		process.env.EDGECTL_API_URLS ?? process.env.NEXT_PUBLIC_EDGECTL_API_URLS;
	const targets = parseHealthTargets(apiTargetList, apiTarget);

	return (
		<main className="p-8">
			<div className="mx-auto flex w-full max-w-2xl flex-col gap-6">
				<HealthStatusCard
					variant="card"
					targets={targets}
					intervalMs={5_000}
					cycleMs={15_000}
				/>
			</div>
		</main>
	);
}
