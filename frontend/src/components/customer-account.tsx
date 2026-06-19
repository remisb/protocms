import { Boxes } from "lucide-react";
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar";

export function CustomerAccount({
  dataset,
  subject,
  role,
}: {
  dataset?: string;
  subject?: string;
  role?: string | null;
}) {
  const subtitle = subject
    ? role
      ? `${subject} · ${role}`
      : subject
    : "ProtoCMS";

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        <SidebarMenuButton
          size="lg"
          className="cursor-default hover:bg-transparent"
        >
          <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <Boxes className="size-4" />
          </div>
          <div className="grid flex-1 text-left leading-tight">
            <span className="truncate text-sm font-semibold">
              {dataset ?? "ProtoCMS"}
            </span>
            <span className="truncate text-xs text-muted-foreground">
              {subtitle}
            </span>
          </div>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  );
}
