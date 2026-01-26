import { HealthStatusCard } from "@/components/health-status-card";
import { PageTabs } from "@/components/page-tabs";
import { parseHealthTargets } from "@/lib/health";

export default function HomePage() {
	const apiTarget =
		process.env.EDGECTL_API_URL ?? process.env.NEXT_PUBLIC_EDGECTL_API_URL;
	const apiTargetList =
		process.env.EDGECTL_API_URLS ?? process.env.NEXT_PUBLIC_EDGECTL_API_URLS;
	const targets = parseHealthTargets(apiTargetList, apiTarget);

	return (
		<PageTabs
			tabs={[
				{
					value: "overview",
					label: "Overview",
					content: (
						<HealthStatusCard
							variant="card"
							targets={targets}
							intervalMs={5_000}
							cycleMs={15_000}
						/>
					),
				},
			]}
		/>
	);
}
