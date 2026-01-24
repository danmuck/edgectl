"use client"

import * as React from "react"
import { ThemeProvider as NextThemesProvider, useTheme } from "next-themes"

type ThemeProviderProps = React.ComponentProps<typeof NextThemesProvider> & {
	storageKey?: string
}

function ThemeCookieSync({ storageKey }: { storageKey: string }) {
	const { theme, resolvedTheme } = useTheme()

	React.useEffect(() => {
		const value = theme === "system" ? resolvedTheme : theme
		if (!value) return
		document.cookie = `${storageKey}=${value}; path=/; max-age=31536000`
	}, [theme, resolvedTheme, storageKey])

	return null
}

export function ThemeProvider({
	children,
	storageKey = "vite-ui-theme",
	...props
}: ThemeProviderProps) {
	return (
		<NextThemesProvider storageKey={storageKey} {...props}>
			<ThemeCookieSync storageKey={storageKey} />
			{children}
		</NextThemesProvider>
	)
}
