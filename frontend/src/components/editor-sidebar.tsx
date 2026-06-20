import { NavLink, useLocation } from "react-router-dom";
import { Database, FileText, Layers } from "lucide-react";
import type { ContentType } from "@/lib/types";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";
import { CustomerAccount } from "@/components/customer-account";

export function EditorSidebar({
  types,
  active,
  counts,
  dataset,
  subject,
  role,
  isAdmin,
  onSelect,
}: {
  types: ContentType[];
  active: string | null;
  counts?: Record<string, number>;
  dataset?: string;
  subject?: string;
  role?: string | null;
  isAdmin: boolean;
  onSelect: (name: string) => void;
}) {
  const { pathname } = useLocation();
  return (
    <Sidebar collapsible="offcanvas">
      <SidebarHeader>
        <CustomerAccount dataset={dataset} subject={subject} role={role} />
      </SidebarHeader>
      <SidebarContent>
        <SidebarGroup>
          <SidebarMenu>
            {isAdmin && (
              <SidebarMenuItem>
                <SidebarMenuButton
                  asChild
                  isActive={pathname.startsWith("/designer")}
                >
                  <NavLink to="/designer">
                    <Layers />
                    <span>Designer</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
            {isAdmin && (
              <SidebarMenuItem>
                <SidebarMenuButton
                  asChild
                  isActive={pathname.startsWith("/datasets")}
                >
                  <NavLink to="/datasets">
                    <Database />
                    <span>Datasets</span>
                  </NavLink>
                </SidebarMenuButton>
              </SidebarMenuItem>
            )}
            <SidebarMenuItem>
              <SidebarMenuButton
                asChild
                isActive={pathname.startsWith("/editor")}
              >
                <NavLink to="/editor">
                  <FileText />
                  <span>Editor</span>
                </NavLink>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroup>
        <SidebarGroup>
          <SidebarGroupLabel>Content types</SidebarGroupLabel>
          <SidebarMenu>
            {types.map((t) => {
              const count = counts?.[t.name];
              return (
                <SidebarMenuItem key={t.name}>
                  <SidebarMenuButton
                    isActive={t.name === active}
                    onClick={() => onSelect(t.name)}
                    className="font-mono"
                  >
                    <span className="truncate">{t.name}</span>
                  </SidebarMenuButton>
                  {count != null && (
                    <SidebarMenuBadge>{count}</SidebarMenuBadge>
                  )}
                </SidebarMenuItem>
              );
            })}
          </SidebarMenu>
        </SidebarGroup>
      </SidebarContent>
    </Sidebar>
  );
}
