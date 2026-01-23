"use client"

import * as React from "react"
import { toast } from "sonner"
import {
  BellIcon,
  CalendarIcon,
  CheckIcon,
  FolderIcon,
  HomeIcon,
  PlusIcon,
  SearchIcon,
  SettingsIcon,
  TriangleAlertIcon,
} from "lucide-react"
import {
  CartesianGrid,
  Line,
  LineChart,
  XAxis,
  YAxis,
} from "recharts"

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Calendar } from "@/components/ui/calendar"
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import {
  ChartContainer,
  ChartLegend,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
} from "@/components/ui/chart"
import {
  Drawer,
  DrawerClose,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from "@/components/ui/drawer"
import {
  Field,
  FieldContent,
  FieldDescription,
  FieldError,
  FieldGroup,
  FieldLabel,
  FieldLegend,
  FieldSeparator,
  FieldSet,
  FieldTitle,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
  InputGroupText,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { Label } from "@/components/ui/label"
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination"
import { Progress } from "@/components/ui/progress"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInput,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarProvider,
  SidebarSeparator,
} from "@/components/ui/sidebar"
import { Skeleton } from "@/components/ui/skeleton"
import { Toaster } from "@/components/ui/sonner"
import { Spinner } from "@/components/ui/spinner"
import { Switch } from "@/components/ui/switch"
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { Toggle } from "@/components/ui/toggle"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"

// Theme overrides for this dev page only. Edit these values to tweak the look.
const themeVars = {
  "--radius": "0.9rem",
  "--background": "220 24% 98%",
  "--foreground": "222 47% 11%",
  "--card": "0 0% 100%",
  "--card-foreground": "222 47% 11%",
  "--popover": "0 0% 100%",
  "--popover-foreground": "222 47% 11%",
  "--primary": "221 83% 53%",
  "--primary-foreground": "210 40% 98%",
  "--secondary": "220 14% 96%",
  "--secondary-foreground": "222 47% 11%",
  "--muted": "220 14% 96%",
  "--muted-foreground": "215 16% 47%",
  "--accent": "216 34% 90%",
  "--accent-foreground": "222 47% 11%",
  "--destructive": "0 84% 60%",
  "--destructive-foreground": "210 40% 98%",
  "--border": "214 32% 91%",
  "--input": "214 32% 91%",
  "--ring": "221 83% 53%",
  "--sidebar": "223 22% 14%",
  "--sidebar-foreground": "210 40% 98%",
  "--sidebar-border": "223 18% 22%",
  "--sidebar-accent": "223 20% 20%",
  "--sidebar-accent-foreground": "210 40% 98%",
  "--sidebar-ring": "221 83% 53%",
} as React.CSSProperties

const chartData = [
  { month: "Jan", desktop: 186, mobile: 80 },
  { month: "Feb", desktop: 305, mobile: 200 },
  { month: "Mar", desktop: 237, mobile: 120 },
  { month: "Apr", desktop: 173, mobile: 190 },
  { month: "May", desktop: 209, mobile: 130 },
  { month: "Jun", desktop: 314, mobile: 240 },
]

const chartConfig = {
  desktop: { label: "Desktop", color: "#2563eb" },
  mobile: { label: "Mobile", color: "#22c55e" },
}

const tableRows = [
  { name: "Edge Node A", status: "Healthy", latency: "12ms" },
  { name: "Edge Node B", status: "Degraded", latency: "87ms" },
  { name: "Edge Node C", status: "Healthy", latency: "20ms" },
]

const scrollItems = Array.from({ length: 12 }, (_, index) => ({
  id: `log-${index + 1}`,
  label: `Log entry ${index + 1}`,
}))

const themeSwatches = [
  { label: "Primary", className: "bg-primary text-primary-foreground" },
  {
    label: "Secondary",
    className: "bg-secondary text-secondary-foreground",
  },
  { label: "Accent", className: "bg-accent text-accent-foreground" },
  {
    label: "Muted",
    className: "bg-muted text-muted-foreground border border-border",
  },
  {
    label: "Destructive",
    className: "bg-destructive text-destructive-foreground",
  },
]

export default function DevPage() {
  const [mounted, setMounted] = React.useState(false)
  const [date, setDate] = React.useState<Date | undefined>(undefined)

  React.useEffect(() => {
    setMounted(true)
    setDate(new Date())
  }, [])

  return (
    <div
      style={themeVars}
      className="min-h-svh bg-background text-foreground"
    >
      <Toaster />
      <div className="relative">
        <div className="mx-auto flex max-w-6xl flex-col gap-8 px-6 py-10">
          <header className="flex flex-wrap items-center justify-between gap-4">
            <div className="space-y-1">
              <p className="text-muted-foreground text-xs uppercase tracking-[0.2em]">
                Dev Playground
              </p>
              <h1 className="text-2xl font-semibold">Component Lab</h1>
              <p className="text-muted-foreground text-sm">
                Use the tabs to review shadcn UI components in one place.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant="secondary" className="gap-1">
                <CheckIcon className="size-3" />
                Theme ready
              </Badge>
              <Button onClick={() => toast("Theme preview toast")}
              >
                Trigger Toast
              </Button>
            </div>
          </header>

          <Card className="border-border/60 bg-card/70 backdrop-blur">
            <CardHeader>
              <CardTitle>Theme overrides (edit in this file)</CardTitle>
              <CardDescription>
                Update the <span className="font-mono">themeVars</span> object at
                the top of <span className="font-mono">app/dev/page.tsx</span> to
                iterate on colors and radius.
              </CardDescription>
              <CardAction>
                <Badge variant="outline">Local theme</Badge>
              </CardAction>
            </CardHeader>
            <CardContent className="grid gap-4">
              <div className="grid gap-2 text-xs text-muted-foreground">
                <span>
                  Tokens preview (primary, secondary, accent, muted, destructive)
                </span>
                <div className="flex flex-wrap gap-2">
                  {themeSwatches.map((swatch) => (
                    <div
                      key={swatch.label}
                      className={`flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium ${swatch.className}`}
                    >
                      <span>{swatch.label}</span>
                    </div>
                  ))}
                </div>
              </div>
              <Separator />
              <div className="grid gap-2 text-xs">
                <div className="rounded-lg border border-dashed border-border bg-muted/40 p-3">
                  Edit the values in <span className="font-mono">themeVars</span>
                  {" "}to update this entire page without touching global styles.
                </div>
              </div>
            </CardContent>
          </Card>

          <Tabs defaultValue="overview" className="space-y-6">
            <TabsList className="flex w-full flex-wrap justify-start gap-2 bg-transparent p-0">
              <TabsTrigger value="overview">Overview</TabsTrigger>
              <TabsTrigger value="forms">Forms</TabsTrigger>
              <TabsTrigger value="data">Data</TabsTrigger>
              <TabsTrigger value="overlays">Overlays</TabsTrigger>
              <TabsTrigger value="navigation">Navigation</TabsTrigger>
            </TabsList>

            <TabsContent value="overview" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Buttons & Badges</CardTitle>
                    <CardDescription>Variants, icons, and states.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4">
                    <div className="flex flex-wrap gap-2">
                      <Button>Primary</Button>
                      <Button variant="secondary">Secondary</Button>
                      <Button variant="outline">Outline</Button>
                      <Button variant="ghost">Ghost</Button>
                      <Button variant="destructive">Destructive</Button>
                      <Button variant="link">Link</Button>
                    </div>
                    <div className="flex flex-wrap items-center gap-2">
                      <Badge>Default</Badge>
                      <Badge variant="secondary">Secondary</Badge>
                      <Badge variant="outline">Outline</Badge>
                      <Spinner />
                    </div>
                    <div className="flex flex-wrap items-center gap-3">
                      <Toggle variant="outline" defaultPressed>
                        Toggle
                      </Toggle>
                      <div className="flex items-center gap-2">
                        <Switch id="switch-demo" defaultChecked />
                        <Label htmlFor="switch-demo">Switch</Label>
                      </div>
                    </div>
                    <div className="grid gap-2">
                      <div className="flex items-center gap-2">
                        <span className="text-sm">Progress</span>
                        <Progress value={66} className="flex-1" />
                      </div>
                      <div className="flex items-center gap-2">
                        <Skeleton className="h-8 w-28" />
                        <Skeleton className="h-8 w-20" />
                        <Skeleton className="h-8 w-16" />
                      </div>
                    </div>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Alerts</CardTitle>
                    <CardDescription>Inline messaging styles.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4">
                    <Alert>
                      <TriangleAlertIcon />
                      <AlertTitle>Heads up</AlertTitle>
                      <AlertDescription>
                        This is a default alert for testing the theme.
                      </AlertDescription>
                    </Alert>
                    <Alert variant="destructive">
                      <TriangleAlertIcon />
                      <AlertTitle>Destructive</AlertTitle>
                      <AlertDescription>
                        Something needs your attention.
                      </AlertDescription>
                    </Alert>
                    <div className="flex items-center gap-2 text-sm text-muted-foreground">
                      <CalendarIcon className="size-4" />
                      Calendar preview is in the Data tab.
                    </div>
                  </CardContent>
                </Card>
              </div>
            </TabsContent>

            <TabsContent value="forms" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Inputs</CardTitle>
                    <CardDescription>Text inputs and textarea.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4">
                    <div className="grid gap-2">
                      <Label htmlFor="email">Email</Label>
                      <Input id="email" placeholder="you@example.com" />
                    </div>
                    <div className="grid gap-2">
                      <Label htmlFor="message">Message</Label>
                      <Textarea id="message" placeholder="Write a note..." />
                    </div>
                  </CardContent>
                  <CardFooter className="justify-end gap-2">
                    <Button variant="outline">Cancel</Button>
                    <Button>Save</Button>
                  </CardFooter>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Input groups</CardTitle>
                    <CardDescription>Prefix, suffix, and textarea.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4">
                    <InputGroup>
                      <InputGroupAddon>
                        <SearchIcon />
                        <InputGroupText>Search</InputGroupText>
                      </InputGroupAddon>
                      <InputGroupInput placeholder="Filter nodes..." />
                      <InputGroupAddon align="inline-end">
                        <InputGroupButton variant="ghost">Go</InputGroupButton>
                      </InputGroupAddon>
                    </InputGroup>
                    <InputGroup>
                      <InputGroupAddon align="block-start">
                        <InputGroupText>Notes</InputGroupText>
                      </InputGroupAddon>
                      <InputGroupTextarea placeholder="Multiline input group..." />
                    </InputGroup>
                  </CardContent>
                </Card>
              </div>

              <Card>
                <CardHeader>
                  <CardTitle>Field layouts</CardTitle>
                  <CardDescription>Grouped fields and errors.</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-4">
                  <FieldSet>
                    <FieldLegend>Preferences</FieldLegend>
                    <FieldGroup>
                      <Field>
                        <FieldLabel htmlFor="node-name">Node name</FieldLabel>
                        <FieldContent>
                          <Input id="node-name" placeholder="edge-node-01" />
                          <FieldDescription>
                            This is shown in the control plane.
                          </FieldDescription>
                          <FieldError errors={[{ message: "Name must be unique." }]} />
                        </FieldContent>
                      </Field>
                      <FieldSeparator>Optional</FieldSeparator>
                      <Field orientation="horizontal">
                        <FieldLabel htmlFor="alerts-toggle">
                          <FieldTitle>Alerts</FieldTitle>
                        </FieldLabel>
                        <FieldContent>
                          <Switch id="alerts-toggle" defaultChecked />
                          <FieldDescription>
                            Receive status notifications.
                          </FieldDescription>
                        </FieldContent>
                      </Field>
                    </FieldGroup>
                  </FieldSet>
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value="data" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Chart</CardTitle>
                    <CardDescription>Recharts with theme tokens.</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <ChartContainer config={chartConfig}>
                      <LineChart data={chartData} margin={{ left: 8, right: 8 }}>
                        <CartesianGrid vertical={false} strokeDasharray="4 4" />
                        <XAxis dataKey="month" tickLine={false} axisLine={false} />
                        <YAxis tickLine={false} axisLine={false} width={32} />
                        <ChartTooltip cursor={false} content={<ChartTooltipContent />} />
                        <ChartLegend content={<ChartLegendContent />} />
                        <Line
                          type="monotone"
                          dataKey="desktop"
                          stroke="var(--color-desktop)"
                          strokeWidth={2}
                          dot={false}
                        />
                        <Line
                          type="monotone"
                          dataKey="mobile"
                          stroke="var(--color-mobile)"
                          strokeWidth={2}
                          dot={false}
                        />
                      </LineChart>
                    </ChartContainer>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Calendar & Logs</CardTitle>
                    <CardDescription>Interactive data widgets.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4 md:grid-cols-[auto_1fr]">
                    <Calendar
                      mode="single"
                      selected={date}
                      onSelect={setDate}
                      className="rounded-md border"
                    />
                    <ScrollArea className="h-56 rounded-md border p-3">
                      <div className="grid gap-2 text-sm">
                        {scrollItems.map((item) => (
                          <div
                            key={item.id}
                            className="flex items-center justify-between"
                          >
                            <span>{item.label}</span>
                            <Badge variant="outline">ok</Badge>
                          </div>
                        ))}
                      </div>
                    </ScrollArea>
                  </CardContent>
                </Card>
              </div>

              <Card>
                <CardHeader>
                  <CardTitle>Table & Pagination</CardTitle>
                  <CardDescription>Data density and lists.</CardDescription>
                </CardHeader>
                <CardContent className="grid gap-4">
                  <Table>
                    <TableCaption>Latest node status.</TableCaption>
                    <TableHeader>
                      <TableRow>
                        <TableHead>Node</TableHead>
                        <TableHead>Status</TableHead>
                        <TableHead>Latency</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {tableRows.map((row) => (
                        <TableRow key={row.name}>
                          <TableCell>{row.name}</TableCell>
                          <TableCell>{row.status}</TableCell>
                          <TableCell>{row.latency}</TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                    <TableFooter>
                      <TableRow>
                        <TableCell colSpan={3}>Showing 3 nodes</TableCell>
                      </TableRow>
                    </TableFooter>
                  </Table>
                  <Pagination>
                    <PaginationContent>
                      <PaginationItem>
                        <PaginationPrevious href="#" />
                      </PaginationItem>
                      <PaginationItem>
                        <PaginationLink href="#" isActive>
                          1
                        </PaginationLink>
                      </PaginationItem>
                      <PaginationItem>
                        <PaginationLink href="#">2</PaginationLink>
                      </PaginationItem>
                      <PaginationItem>
                        <PaginationEllipsis />
                      </PaginationItem>
                      <PaginationItem>
                        <PaginationNext href="#" />
                      </PaginationItem>
                    </PaginationContent>
                  </Pagination>
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value="overlays" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Tooltips & Toasts</CardTitle>
                    <CardDescription>Quick feedback helpers.</CardDescription>
                  </CardHeader>
                  <CardContent className="flex flex-wrap items-center gap-2">
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <Button variant="outline">Hover me</Button>
                      </TooltipTrigger>
                      <TooltipContent>Tooltip content</TooltipContent>
                    </Tooltip>
                    <Button variant="secondary" onClick={() => toast("Toast from overlays tab")}
                    >
                      Trigger toast
                    </Button>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Alert dialog</CardTitle>
                    <CardDescription>Confirmation patterns.</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <AlertDialog>
                      <AlertDialogTrigger asChild>
                        <Button variant="secondary">Open Alert</Button>
                      </AlertDialogTrigger>
                      <AlertDialogContent>
                        <AlertDialogHeader>
                          <AlertDialogTitle>Confirm action</AlertDialogTitle>
                          <AlertDialogDescription>
                            This is a demo alert dialog for styling.
                          </AlertDialogDescription>
                        </AlertDialogHeader>
                        <AlertDialogFooter>
                          <AlertDialogCancel>Cancel</AlertDialogCancel>
                          <AlertDialogAction>Continue</AlertDialogAction>
                        </AlertDialogFooter>
                      </AlertDialogContent>
                    </AlertDialog>
                  </CardContent>
                </Card>
              </div>

              <Card>
                <CardHeader>
                  <CardTitle>Sheet & Drawer</CardTitle>
                  <CardDescription>Off-canvas components.</CardDescription>
                </CardHeader>
                <CardContent className="flex flex-wrap gap-2">
                  <Sheet>
                    <SheetTrigger asChild>
                      <Button variant="outline">Open Sheet</Button>
                    </SheetTrigger>
                    <SheetContent side="right">
                      <SheetHeader>
                        <SheetTitle>Sheet title</SheetTitle>
                        <SheetDescription>
                          This is a demo sheet for styling.
                        </SheetDescription>
                      </SheetHeader>
                      <div className="grid gap-2 px-4 text-sm">
                        <Label htmlFor="sheet-input">Label</Label>
                        <Input id="sheet-input" placeholder="Value" />
                      </div>
                      <SheetFooter>
                        <SheetClose asChild>
                          <Button variant="outline">Close</Button>
                        </SheetClose>
                        <Button>Save</Button>
                      </SheetFooter>
                    </SheetContent>
                  </Sheet>

                  <Drawer>
                    <DrawerTrigger asChild>
                      <Button variant="secondary">Open Drawer</Button>
                    </DrawerTrigger>
                    <DrawerContent>
                      <DrawerHeader>
                        <DrawerTitle>Drawer title</DrawerTitle>
                        <DrawerDescription>
                          This is a demo drawer for styling.
                        </DrawerDescription>
                      </DrawerHeader>
                      <div className="px-4 pb-4 text-sm">
                        Drawer content goes here.
                      </div>
                      <DrawerFooter>
                        <DrawerClose asChild>
                          <Button variant="outline">Cancel</Button>
                        </DrawerClose>
                        <Button>Continue</Button>
                      </DrawerFooter>
                    </DrawerContent>
                  </Drawer>
                </CardContent>
              </Card>
            </TabsContent>

            <TabsContent value="navigation" className="space-y-6">
              <div className="grid gap-6 lg:grid-cols-2">
                <Card>
                  <CardHeader>
                    <CardTitle>Tabs & Accordion</CardTitle>
                    <CardDescription>Navigation primitives.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-6">
                    <Tabs defaultValue="overview">
                      <TabsList>
                        <TabsTrigger value="overview">Overview</TabsTrigger>
                        <TabsTrigger value="metrics">Metrics</TabsTrigger>
                      </TabsList>
                      <TabsContent value="overview">
                        <p className="text-muted-foreground text-sm">
                          Tab content for overview.
                        </p>
                      </TabsContent>
                      <TabsContent value="metrics">
                        <p className="text-muted-foreground text-sm">
                          Tab content for metrics.
                        </p>
                      </TabsContent>
                    </Tabs>
                    <Accordion type="single" collapsible defaultValue="item-1">
                      <AccordionItem value="item-1">
                        <AccordionTrigger>What is edgectl?</AccordionTrigger>
                        <AccordionContent>
                          A local control plane for edge deployments.
                        </AccordionContent>
                      </AccordionItem>
                      <AccordionItem value="item-2">
                        <AccordionTrigger>Is this a demo?</AccordionTrigger>
                        <AccordionContent>
                          Yes. It is used to preview the theme.
                        </AccordionContent>
                      </AccordionItem>
                    </Accordion>
                  </CardContent>
                </Card>

                <Card>
                  <CardHeader>
                    <CardTitle>Separator & Status</CardTitle>
                    <CardDescription>Small layout helpers.</CardDescription>
                  </CardHeader>
                  <CardContent className="grid gap-4">
                    <div className="grid gap-2">
                      <div className="flex items-center justify-between text-sm">
                        <span>Build</span>
                        <Badge variant="outline">v0.7.0</Badge>
                      </div>
                      <Separator />
                      <div className="flex items-center justify-between text-sm">
                        <span>Region</span>
                        <Badge variant="secondary">Local</Badge>
                      </div>
                    </div>
                    <div className="flex items-center gap-3">
                      <Button size="sm" variant="outline">
                        <PlusIcon className="size-4" />
                        Add
                      </Button>
                      <Button size="sm">
                        <CheckIcon className="size-4" />
                        Apply
                      </Button>
                    </div>
                  </CardContent>
                </Card>
              </div>

              <SidebarProvider defaultOpen>
                <div className="overflow-hidden rounded-xl border bg-background">
                  <div className="grid md:grid-cols-[260px_1fr]">
                    <Sidebar collapsible="none">
                      <SidebarHeader>
                        <div className="flex items-center gap-2 px-2 py-1">
                          <div className="bg-primary size-2 rounded-full" />
                          <span className="text-sm font-semibold">edgectl</span>
                        </div>
                        <SidebarInput placeholder="Search" />
                      </SidebarHeader>
                      <SidebarSeparator />
                      <SidebarContent>
                        <SidebarGroup>
                          <SidebarGroupLabel>Platform</SidebarGroupLabel>
                          <SidebarGroupAction aria-label="Create">
                            <PlusIcon />
                          </SidebarGroupAction>
                          <SidebarGroupContent>
                            <SidebarMenu>
                              <SidebarMenuItem>
                                <SidebarMenuButton isActive tooltip="Dashboard">
                                  <HomeIcon />
                                  <span>Dashboard</span>
                                </SidebarMenuButton>
                                <SidebarMenuAction showOnHover aria-label="Notifications">
                                  <BellIcon />
                                </SidebarMenuAction>
                                <SidebarMenuBadge>4</SidebarMenuBadge>
                              </SidebarMenuItem>
                              <SidebarMenuItem>
                                <SidebarMenuButton tooltip="Projects">
                                  <FolderIcon />
                                  <span>Projects</span>
                                </SidebarMenuButton>
                                <SidebarMenuSub>
                                  <SidebarMenuSubItem>
                                    <SidebarMenuSubButton href="#">
                                      Alpha
                                    </SidebarMenuSubButton>
                                  </SidebarMenuSubItem>
                                  <SidebarMenuSubItem>
                                    <SidebarMenuSubButton isActive href="#">
                                      Beta
                                    </SidebarMenuSubButton>
                                  </SidebarMenuSubItem>
                                </SidebarMenuSub>
                              </SidebarMenuItem>
                              <SidebarMenuItem>
                                <SidebarMenuButton tooltip="Settings">
                                  <SettingsIcon />
                                  <span>Settings</span>
                                </SidebarMenuButton>
                              </SidebarMenuItem>
                            </SidebarMenu>
                          </SidebarGroupContent>
                        </SidebarGroup>
                        <SidebarSeparator />
                        <SidebarGroup>
                          <SidebarGroupLabel>Loading</SidebarGroupLabel>
                          <SidebarGroupContent>
                            <SidebarMenu>
                              <SidebarMenuItem>
                                {mounted ? <SidebarMenuSkeleton showIcon /> : null}
                              </SidebarMenuItem>
                            </SidebarMenu>
                          </SidebarGroupContent>
                        </SidebarGroup>
                      </SidebarContent>
                      <SidebarFooter>
                        <Button variant="outline" className="w-full">
                          Upgrade
                        </Button>
                      </SidebarFooter>
                    </Sidebar>
                    <div className="flex flex-col gap-4 p-4">
                      <div>
                        <h3 className="text-lg font-semibold">Main content</h3>
                        <p className="text-muted-foreground text-sm">
                          Sidebar layout preview with a static layout.
                        </p>
                      </div>
                      <Card className="border-dashed">
                        <CardHeader>
                          <CardTitle>Inset content card</CardTitle>
                          <CardDescription>
                            Use this area to test sidebar spacing.
                          </CardDescription>
                        </CardHeader>
                        <CardContent className="text-sm text-muted-foreground">
                          The sidebar is rendered with <span className="font-mono">collapsible="none"</span> to
                          avoid fixed positioning in this preview.
                        </CardContent>
                      </Card>
                    </div>
                  </div>
                </div>
              </SidebarProvider>
            </TabsContent>
          </Tabs>
        </div>
      </div>
    </div>
  )
}
