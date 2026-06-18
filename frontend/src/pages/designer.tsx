import { useCallback, useEffect, useState } from "react";
import { Pencil, Plus, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { ContentType } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { CreateContentTypeDialog } from "@/components/create-content-type-dialog";

export function DesignerPage() {
  const [types, setTypes] = useState<ContentType[] | null>(null);
  const [open, setOpen] = useState(false);
  const [editing, setEditing] = useState<ContentType | null>(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    api
      .listContentTypes()
      .then((list) =>
        setTypes(list.sort((a, b) => a.name.localeCompare(b.name))),
      )
      .catch((e: Error) => toast.error(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => load(), [load]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Designer</h1>
          <p className="text-sm text-muted-foreground">
            Define the schema for each content type. Field changes take effect
            immediately for new items.
          </p>
        </div>
        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={load} disabled={loading}>
            <RefreshCw className={loading ? "animate-spin" : ""} />
            Refresh
          </Button>
          <Button
            size="sm"
            onClick={() => {
              setEditing(null);
              setOpen(true);
            }}
          >
            <Plus />
            New content type
          </Button>
        </div>
      </div>

      {types === null ? (
        <SkeletonGrid />
      ) : types.length === 0 ? (
        <EmptyState
          onCreate={() => {
            setEditing(null);
            setOpen(true);
          }}
        />
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {types.map((t) => (
            <ContentTypeCard
              key={t.name}
              type={t}
              onEdit={() => {
                setEditing(t);
                setOpen(true);
              }}
            />
          ))}
        </div>
      )}

      <CreateContentTypeDialog
        open={open}
        onOpenChange={setOpen}
        existingNames={types?.map((t) => t.name) ?? []}
        initial={editing}
        onSaved={() => {
          load();
          toast.success(editing ? "Content type updated" : "Content type created");
        }}
      />
    </div>
  );
}

function ContentTypeCard({
  type,
  onEdit,
}: {
  type: ContentType;
  onEdit: () => void;
}) {
  const fields = Object.entries(type.fields ?? {});
  return (
    <Card>
      <CardHeader className="flex-row items-start justify-between space-y-0">
        <div className="space-y-1">
          <CardTitle className="font-mono text-base">{type.name}</CardTitle>
          <CardDescription>
            {fields.length} {fields.length === 1 ? "field" : "fields"}
          </CardDescription>
        </div>
        <Button
          size="icon"
          variant="ghost"
          onClick={onEdit}
          aria-label="Edit content type"
        >
          <Pencil />
        </Button>
      </CardHeader>
      <CardContent>
        {fields.length === 0 ? (
          <p className="text-sm text-muted-foreground">No fields defined.</p>
        ) : (
          <ul className="space-y-1.5">
            {fields.map(([name, def]) => (
              <li key={name} className="flex items-center justify-between text-sm">
                <span className="font-mono">{name}</span>
                <div className="flex items-center gap-1.5">
                  {def.required && (
                    <Badge variant="destructive" className="text-[10px]">
                      required
                    </Badge>
                  )}
                  <Badge variant="secondary" className="text-[10px]">
                    {def.type}
                    {def.type === "reference" && def.refType
                      ? `: ${def.refType}`
                      : ""}
                  </Badge>
                </div>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <Card>
      <CardContent className="flex flex-col items-center justify-center gap-3 py-16 text-center">
        <h3 className="text-lg font-medium">No content types yet</h3>
        <p className="max-w-sm text-sm text-muted-foreground">
          Define a content type to start authoring items. You can add fields
          like text, number, references, and more.
        </p>
        <Button onClick={onCreate}>
          <Plus />
          Create the first one
        </Button>
      </CardContent>
    </Card>
  );
}

function SkeletonGrid() {
  return (
    <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
      {Array.from({ length: 3 }).map((_, i) => (
        <Card key={i}>
          <CardHeader>
            <div className="h-5 w-24 animate-pulse rounded bg-muted" />
            <div className="h-3 w-16 animate-pulse rounded bg-muted" />
          </CardHeader>
          <CardContent>
            <div className="space-y-2">
              <div className="h-3 w-full animate-pulse rounded bg-muted" />
              <div className="h-3 w-3/4 animate-pulse rounded bg-muted" />
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
}
