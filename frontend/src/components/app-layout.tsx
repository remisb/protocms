import { useEffect, useMemo, useState } from "react";
import { Outlet, useLocation, useNavigate } from "react-router-dom";
import { Boxes, LogOut } from "lucide-react";
import { api } from "@/lib/api";
import { useAuth } from "@/lib/auth";
import type { ContentType, DatasetStats } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { EditorSidebar } from "@/components/editor-sidebar";

export function AppLayout() {
  const { user, role, isAdmin, logout } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const [stats, setStats] = useState<DatasetStats | null>(null);
  const [types, setTypes] = useState<ContentType[] | null>(null);

  useEffect(() => {
    let alive = true;
    api.getStats().then(
      (s) => alive && setStats(s),
      () => {},
    );
    api.listContentTypes().then(
      (list) =>
        alive && setTypes(list.sort((a, b) => a.name.localeCompare(b.name))),
      () => {},
    );
    return () => {
      alive = false;
    };
  }, []);

  // The active content type is the `/editor/:contentType` URL segment.
  const activeType = useMemo(() => {
    const m = location.pathname.match(/^\/editor\/([^/]+)/);
    return m ? decodeURIComponent(m[1]) : null;
  }, [location.pathname]);

  return (
    <SidebarProvider>
      <EditorSidebar
        types={types ?? []}
        active={activeType}
        counts={stats?.items_per_type}
        dataset={stats?.dataset}
        subject={user?.subject}
        role={role}
        isAdmin={isAdmin}
        onSelect={(n) => navigate(`/editor/${n}`)}
      />
      <SidebarInset>
        <header className="flex h-14 shrink-0 items-center gap-4 border-b px-6">
          <SidebarTrigger />
          <div className="flex items-center gap-2 font-semibold">
            <Boxes className="size-5" />
            <span>ProtoCMS</span>
          </div>
          <div className="ml-auto flex items-center gap-2 text-sm text-muted-foreground">
            {stats ? (
              <Badge variant="secondary">
                {stats.content_types} types · {stats.total_items} items
              </Badge>
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
        </header>
        <main className="px-6 py-8">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
