"use client";

import { useState } from "react";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { cn } from "@/lib/utils";

type PageTab = {
	value: string;
	label: string;
	content: React.ReactNode;
};

type PageTabsProps = {
	tabs: PageTab[];
	defaultValue?: string;
	className?: string;
	listClassName?: string;
	contentClassName?: string;
	lazy?: boolean;
};

export function PageTabs({
	tabs,
	defaultValue,
	className,
	listClassName,
	contentClassName,
	lazy = false,
}: PageTabsProps) {
	const initial = defaultValue ?? tabs[0]?.value ?? "default";
	const [activeValue, setActiveValue] = useState(initial);

	return (
		<Tabs
			defaultValue={initial}
			className={cn("w-full", className)}
			onValueChange={setActiveValue}
		>
			<TabsList
				className={cn(
					"bg-white/5 text-slate-200 border border-white/10 shadow-sm",
					listClassName,
				)}
			>
				{tabs.map((tab) => (
					<TabsTrigger key={tab.value} value={tab.value}>
						{tab.label}
					</TabsTrigger>
				))}
			</TabsList>
			{tabs.map((tab) =>
				lazy && tab.value !== activeValue ? null : (
					<TabsContent
						key={tab.value}
						value={tab.value}
						className={cn("mt-6", contentClassName)}
					>
						{tab.content}
					</TabsContent>
				),
			)}
		</Tabs>
	);
}
