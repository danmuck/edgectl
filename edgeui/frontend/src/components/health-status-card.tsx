"use client";

import { useEffect, useMemo, useState } from "react";
import {
	Activity,
	AlertTriangle,
	ChevronLeft,
	ChevronRight,
	RadioTower,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { formatGoDuration } from "@/lib/format-go-duration";
import type { HealthTarget } from "@/lib/health";
import { useHealthPolling } from "@/hooks/use-health-polling";
import { cn } from "@/lib/utils";

type HealthStatusCardVariant = "nav" | "card";

type HealthStatusCardProps = {
	targets: HealthTarget[];
	variant?: HealthStatusCardVariant;
	intervalMs?: number;
	cycleMs?: number;
	showControls?: boolean;
	className?: string;
};

export function HealthStatusCard({
	targets,
	variant = "card",
	intervalMs = 5_000,
	cycleMs,
	showControls = true,
	className,
}: HealthStatusCardProps) {
	const safeTargets = useMemo(
		() => (targets.length ? targets : []),
		[targets],
	);
	const [activeIndex, setActiveIndex] = useState(0);

	useEffect(() => {
		if (activeIndex >= safeTargets.length) {
			setActiveIndex(0);
		}
	}, [activeIndex, safeTargets.length]);

	useEffect(() => {
		if (!cycleMs || safeTargets.length < 2) return undefined;
		const timer = window.setInterval(() => {
			setActiveIndex((current) => (current + 1) % safeTargets.length);
		}, cycleMs);
		return () => window.clearInterval(timer);
	}, [cycleMs, safeTargets.length]);

	const activeTarget = safeTargets[activeIndex];
	const { health, error } = useHealthPolling(activeTarget?.apiUrl, {
		intervalMs,
	});

	const hasManyTargets = safeTargets.length > 1;
	const showNavControls = showControls && hasManyTargets;
	const isHealthy = health?.status === "ok";
	const uptimeLabel = health?.uptime
		? formatGoDuration(health.uptime)
		: "Uptime unavailable";
	const versionLabel = health?.version ? `v${health.version}` : null;
	const displayLabel =
		hasManyTargets && activeTarget?.label
			? activeTarget.label
			: health?.service ?? activeTarget?.label ?? "edge-api";
	const apiHost = activeTarget?.apiUrl
		? formatApiTarget(activeTarget.apiUrl)
		: "API target unavailable";

	const containerClass =
		variant === "card"
			? "rounded-2xl border border-white/10 bg-gradient-to-br from-slate-950 via-slate-900 to-slate-950 p-6 text-slate-100 shadow-xl"
			: "flex flex-col gap-2 text-xs text-slate-100";
	const alignmentClass =
		variant === "card" ? "items-start text-left" : "items-end text-right";

	return (
		<div className={cn(containerClass, alignmentClass, className)}>
			<div className="flex w-full items-center justify-between gap-3">
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
						? `${displayLabel}: Healthy`
						: `${displayLabel}: Down`}
				</Badge>
				{showNavControls && (
					<div className="flex items-center gap-1">
						<Button
							type="button"
							variant="ghost"
							size="icon"
							className="size-6 text-slate-200 hover:bg-white/10"
							onClick={() =>
								setActiveIndex(
									(current) =>
										(current - 1 + safeTargets.length) %
										safeTargets.length,
								)
							}
							aria-label="Previous server"
						>
							<ChevronLeft className="size-3" />
						</Button>
						<Button
							type="button"
							variant="ghost"
							size="icon"
							className="size-6 text-slate-200 hover:bg-white/10"
							onClick={() =>
								setActiveIndex(
									(current) => (current + 1) % safeTargets.length,
								)
							}
							aria-label="Next server"
						>
							<ChevronRight className="size-3" />
						</Button>
					</div>
				)}
			</div>
			<div
				className={cn(
					"flex w-full flex-col gap-1 text-xs",
					variant === "card" ? "pt-3" : "pt-1",
				)}
			>
				<div className="flex items-center gap-2 text-slate-300">
					<RadioTower className="size-3 text-slate-400" />
					<span>{apiHost}</span>
				</div>
				{error && (
					<div className="flex items-center gap-2 text-amber-200">
						<AlertTriangle className="size-3" />
						<span>{error}</span>
					</div>
				)}
			</div>
			<div className="flex w-full flex-wrap items-center justify-between gap-2 text-xs text-slate-300">
				{versionLabel && (
					<span className="font-mono text-[11px] text-slate-300">
						{versionLabel}
					</span>
				)}
				<span className="shrink-0 text-muted-foreground">{uptimeLabel}</span>
			</div>
		</div>
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
