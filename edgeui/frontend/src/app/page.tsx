import { fetchHealth } from "@/lib/health";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import Link from "next/link";

export default async function HomePage() {
	let health;

	try {
		health = await fetchHealth();
	} catch (err) {
		console.error("fetchHealth failed:", err);
		health = { status: "down" };
	}

	const isUp = health.status === "ok";

	return (
		<main className="p-8">
			<Card className="max-w-md">
				<CardHeader>
					<CardTitle>API Health</CardTitle>
				</CardHeader>
				<CardContent className="space-y-2">
					<div className="flex items-center gap-2">
						<span>Status:</span>
						<Badge variant={isUp ? "default" : "destructive"}>
							{isUp ? "Healthy" : "Down"}
						</Badge>
					</div>

					{health.version && (
						<div className="text-sm text-muted-foreground">
							Version: {health.version}
						</div>
					)}

					{health.uptime && (
						<div className="text-sm text-muted-foreground">
							Uptime: {health.uptime}
						</div>
					)}
				</CardContent>
			</Card>
		</main>
	);
}
