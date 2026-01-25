import Link from "next/link";
import {
	Activity,
	AlertTriangle,
	ArrowUpRight,
	RadioTower,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { fetchHealth, type HealthResponse } from "@/lib/health";
import { ThemeToggleButton } from "@/components/theme-toggle-button";
import { formatGoDuration } from "@/lib/format-go-duration";

const navLinks: Array<{ href: string; label: string }> = [
	{ href: "/", label: "Status" },
	{ href: "/dev", label: "Component Lab" },
	{ href: "/tmp", label: "Temp" },
];

export default async function DevNav() {
	const apiTarget = process.env.EDGECTL_API_URL;
	const environment =
		process.env.NEXT_PUBLIC_EDGECTL_ENV ??
		process.env.VERCEL_ENV ??
		process.env.NODE_ENV ??
		"local";

	let health: HealthResponse | null = null;
	let healthError: string | null = null;

	if (apiTarget) {
		try {
			health = await fetchHealth();
		} catch (err) {
			healthError =
				err instanceof Error ? err.message : "Health check unavailable";
		}
	} else {
		healthError = "EDGECTL_API_URL is not configured";
	}

	const isHealthy = health?.status === "ok";
	const uptimeLabel = health?.uptime
		? formatGoDuration(health.uptime)
		: "Uptime unavailable";
	const versionLabel = health?.version ? `v${health.version}` : null;
	const serviceLabel = health?.service ? `${health.service}` : null;
	const apiHost = apiTarget ? formatApiTarget(apiTarget) : null;

	return (
		<header className="relative z-50 border-b border-white/10 bg-gradient-to-r from-slate-950 via-slate-900 to-slate-950 text-slate-100 shadow-lg">
			{/* Main Nav container */}
			<div className="mx-auto flex w-full max-w-6xl flex-wrap items-center gap-4 px-6 py-4">
				{/* Title card */}
				<div className="flex items-center gap-3">
					<span className="text-xs font-semibold uppercase tracking-[0.35em] text-slate-400">
						edgectl
					</span>
					<Badge
						variant="secondary"
						className="bg-white/10 text-white hover:bg-white/20"
					>
						{environment}
					</Badge>
				</div>
				{/* Nav links */}
				<nav className="flex flex-1 flex-wrap items-center justify-start gap-2 text-sm sm:justify-center">
					{navLinks.map((link) => (
						<Button
							key={link.href}
							variant="ghost"
							size="sm"
							asChild
							className="text-slate-100 hover:bg-white/10"
						>
							<Link href={link.href}>{link.label}</Link>
						</Button>
					))}
					<Button
						variant="outline"
						size="sm"
						className="border-white/30 text-slate-100 hover:bg-white/10"
						asChild
					>
						<a
							href="https://github.com/danmuck/edgectl"
							target="_blank"
							rel="noreferrer"
						>
							Docs
							<ArrowUpRight className="size-3" />
						</a>
					</Button>
				</nav>
				{/* Health Check */}
				<div className="flex flex-col items-start gap-1 text-xs sm:items-end">
					<div className="flex items-center gap-2">
						<Badge
							variant={isHealthy ? "default" : "destructive"}
							className={
								isHealthy
									? "bg-emerald-500 text-emerald-950 hover:bg-emerald-500/90"
									: undefined
							}
						>
							<Activity className="size-3" />
							{isHealthy
								? `${serviceLabel}: Healthy`
								: `${serviceLabel}: Down`}
						</Badge>
					</div>
					<div className="flex items-center justify-between w-full">
						{versionLabel && (
							<span className="font-mono text-[11px] text-slate-300">
								{versionLabel}
							</span>
						)}
						<span className="shrink-0 text-muted-foreground">
							{uptimeLabel}
						</span>
					</div>
				</div>
				<ThemeToggleButton />
			</div>
			<div className="mx-auto flex w-full max-w-6xl flex-wrap items-center gap-3 border-t border-white/10 px-6 py-3 text-xs text-slate-300">
				<div className="flex items-center gap-2">
					<RadioTower className="size-3 text-slate-400" />
					<span>{apiHost ?? "API target unavailable"}</span>
				</div>
				{healthError && (
					<div className="flex items-center gap-2 text-amber-200">
						<AlertTriangle className="size-3" />
						<span>{healthError}</span>
					</div>
				)}
			</div>
			<div className="pointer-events-none absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-white/40 to-transparent" />
		</header>
	);
}

function formatApiTarget(url: string) {
	try {
		const parsed = new URL(url);
		return parsed.host;
	} catch {
		return url;
	}
}
