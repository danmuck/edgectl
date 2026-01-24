"use client";

import { useEffect, useState } from "react";
import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

import { Button } from "@/components/ui/button";

export function ThemeToggleButton() {
	const { resolvedTheme, setTheme } = useTheme();
	const [mounted, setMounted] = useState(false);

	useEffect(() => {
		setMounted(true);
	}, []);

	const currentTheme = resolvedTheme ?? "system";
	const isDark = currentTheme === "dark";
	const label = isDark ? "Switch to light mode" : "Switch to dark mode";

	if (!mounted) {
		return (
			<Button
				variant="ghost"
				size="icon"
				className="h-9 w-9 text-slate-100 dark:hover:bg-white/10 light:hover:bg-black/10"
				aria-label="Toggle color scheme"
				disabled
			>
				<Sun className="size-4" />
			</Button>
		);
	}

	return (
		<Button
			variant="ghost"
			size="icon"
			className="relative h-9 w-9 text-slate-100 hover:bg-white/10 hover:text-white"
			onClick={() => setTheme(isDark ? "light" : "dark")}
			aria-label={label}
		>
			<Sun
				className={`size-4 transition-opacity ${isDark ? "opacity-0" : "opacity-100"}`}
			/>
			<Moon
				className={`absolute size-4 transition-opacity ${isDark ? "opacity-100" : "opacity-0"}`}
			/>
		</Button>
	);
}
