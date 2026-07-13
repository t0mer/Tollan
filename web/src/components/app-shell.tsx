import { useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import {
  Activity,
  Bell,
  GitBranch,
  LayoutDashboard,
  Menu,
  Radio,
  Search,
  Server,
  Settings,
  Waypoints,
  X,
} from "lucide-react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { cn } from "@/lib/utils";
import { ThemeToggle } from "@/components/theme-toggle";

type NavItem = { to: string; label: string; icon: React.ComponentType<{ className?: string }> };

const NAV: NavItem[] = [
  { to: "/", label: "Overview", icon: Activity },
  { to: "/search", label: "Search", icon: Search },
  { to: "/streams", label: "Streams", icon: Waypoints },
  { to: "/pipelines", label: "Pipelines", icon: GitBranch },
  { to: "/dashboards", label: "Dashboards", icon: LayoutDashboard },
  { to: "/alerts", label: "Alerts", icon: Bell },
  { to: "/inputs", label: "Inputs", icon: Radio },
  { to: "/fleet", label: "Fleet", icon: Server },
  { to: "/system", label: "System", icon: Settings },
];

function Brand() {
  return (
    <div className="flex items-center gap-2.5">
      {/* Jade obsidian mark: a stacked-glyph nod to the Toltec city's masonry. */}
      <span className="grid h-8 w-8 place-items-center rounded-md bg-primary text-primary-foreground">
        <span className="font-display text-lg font-bold leading-none">T</span>
      </span>
      <span className="font-display text-lg font-semibold tracking-tight">Tollan</span>
    </div>
  );
}

function NavItems({ onNavigate }: { onNavigate?: () => void }) {
  return (
    <nav className="flex flex-col gap-1">
      {NAV.map(({ to, label, icon: Icon }) => (
        <NavLink
          key={to}
          to={to}
          end={to === "/"}
          onClick={onNavigate}
          className={({ isActive }) =>
            cn(
              "flex items-center gap-3 rounded-md px-3 py-2 text-sm font-medium transition-colors",
              isActive
                ? "bg-accent text-accent-foreground"
                : "text-muted-foreground hover:bg-muted hover:text-foreground",
            )
          }
        >
          <Icon className="size-4" />
          {label}
        </NavLink>
      ))}
    </nav>
  );
}

function VersionFooter() {
  const { data } = useQuery({ queryKey: ["version"], queryFn: api.version });
  return (
    <div className="mt-auto px-3 pt-4 font-mono text-xs text-muted-foreground">
      {data ? `v${data.version}` : "—"}
    </div>
  );
}

export function AppShell() {
  const [mobileOpen, setMobileOpen] = useState(false);

  return (
    <div className="flex min-h-screen">
      {/* Desktop sidebar */}
      <aside className="hidden w-60 shrink-0 flex-col border-r border-border bg-card px-4 py-5 md:flex">
        <Brand />
        <div className="mt-8 flex flex-1 flex-col">
          <NavItems />
          <VersionFooter />
        </div>
      </aside>

      {/* Mobile drawer */}
      {mobileOpen && (
        <div className="fixed inset-0 z-40 md:hidden">
          <div
            className="absolute inset-0 bg-black/50"
            onClick={() => setMobileOpen(false)}
            aria-hidden
          />
          <aside className="absolute inset-y-0 left-0 flex w-64 flex-col border-r border-border bg-card px-4 py-5">
            <div className="flex items-center justify-between">
              <Brand />
              <button
                aria-label="Close menu"
                onClick={() => setMobileOpen(false)}
                className="rounded-md p-1 text-muted-foreground hover:bg-muted"
              >
                <X className="size-5" />
              </button>
            </div>
            <div className="mt-8 flex flex-1 flex-col">
              <NavItems onNavigate={() => setMobileOpen(false)} />
              <VersionFooter />
            </div>
          </aside>
        </div>
      )}

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-3 border-b border-border bg-background/80 px-4 backdrop-blur">
          <button
            aria-label="Open menu"
            onClick={() => setMobileOpen(true)}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-muted md:hidden"
          >
            <Menu className="size-5" />
          </button>
          <div className="ml-auto">
            <ThemeToggle />
          </div>
        </header>
        <main className="min-w-0 flex-1 p-4 md:p-8">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
