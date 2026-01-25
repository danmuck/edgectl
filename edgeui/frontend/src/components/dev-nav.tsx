"use client";

import Link from "next/link";
import { ArrowUpRight } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { ThemeToggleButton } from "@/components/theme-toggle-button";
import { HealthStatusCard } from "@/components/health-status-card";
import { parseHealthTargets } from "@/lib/health";

const navLinks: Array<{ href: string; label: string }> = [
	{ href: "/", label: "Status" },
	{ href: "/dev", label: "Component Lab" },
	{ href: "/tmp", label: "Temp" },
];

export default function DevNav() {
	const apiTarget = process.env.NEXT_PUBLIC_EDGECTL_API_URL;
	const apiTargetList = process.env.NEXT_PUBLIC_EDGECTL_API_URLS;
	const environment =
		process.env.NEXT_PUBLIC_EDGECTL_ENV ??
		process.env.VERCEL_ENV ??
		process.env.NODE_ENV ??
		"local";
	const targets = parseHealthTargets(apiTargetList, apiTarget);

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
				<HealthStatusCard
					variant="nav"
					targets={targets}
					intervalMs={5_000}
					cycleMs={15_000}
					className="min-w-[220px]"
				/>
				<ThemeToggleButton />
			</div>
			<div className="pointer-events-none absolute inset-x-0 bottom-0 h-px bg-gradient-to-r from-transparent via-white/40 to-transparent" />
		</header>
	);
}
