import { useEffect, useState } from "react";
import { NavLink, Outlet } from "react-router-dom";
import { Boxes, FileText, Layers, LogOut } from "lucide-react";
import { cn } from "@/lib/utils";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { DatasetStats } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

export function AppLayout() {
  const { user, role, logout } = useAuth();
  const [stats, setStats] = useState<DatasetStats | null>(null);

  useEffect(() => {
    let alive = true;
    api.getStats().then(
      (s) => alive && setStats(s),
      () => {},
    );
    return () => {
      alive = false;
    };
  }, []);

  return (
    <div className="min-h-screen bg-background">
      <header className="border-b">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-6 px-6">
          <div className="flex items-center gap-2 font-semibold">
            <Boxes className="size-5" />
            <span>ProtoCMS</span>
          </div>
          <nav className="flex items-center gap-1">
            <NavTab to="/designer" icon={<Layers className="size-4" />}>
              Designer
            </NavTab>
            <NavTab to="/editor" icon={<FileText className="size-4" />}>
              Editor
            </NavTab>
          </nav>
          <div className="ml-auto flex items-center gap-2 text-sm text-muted-foreground">
            {stats ? (
              <>
                <Badge variant="outline">dataset: {stats.dataset}</Badge>
                <Badge variant="secondary">
                  {stats.content_types} types · {stats.total_items} items
                </Badge>
              </>
            ) : (
              <span className="text-xs">connecting…</span>
            )}
            {user && (
              <Badge variant="outline">
                {user.subject}
                {role ? ` · ${role}` : ""}
              </Badge>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={logout}
              aria-label="Log out"
            >
              <LogOut />
              Logout
            </Button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-7xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  );
}

function NavTab({
  to,
  icon,
  children,
}: {
  to: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <NavLink
      to={to}
      className={({ isActive }) =>
        cn(
          "flex items-center gap-2 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
          isActive
            ? "bg-accent text-accent-foreground"
            : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
        )
      }
    >
      {icon}
      {children}
    </NavLink>
  );
}
