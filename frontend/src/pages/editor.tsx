import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Eye, Pencil, Plus, RefreshCw, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { ContentItem, ContentType } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { ContentItemDialog } from "@/components/content-item-dialog";
import { ViewContentItemDialog } from "@/components/view-content-item-dialog";

export function EditorPage() {
  const { contentType } = useParams<{ contentType?: string }>();
  const navigate = useNavigate();

  const [types, setTypes] = useState<ContentType[] | null>(null);
  const [items, setItems] = useState<ContentItem[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [editing, setEditing] = useState<ContentItem | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [viewing, setViewing] = useState<ContentItem | null>(null);
  const [viewOpen, setViewOpen] = useState(false);

  const activeType = useMemo(
    () => types?.find((t) => t.name === contentType) ?? null,
    [types, contentType],
  );

  useEffect(() => {
    api
      .listContentTypes()
      .then((list) =>
        setTypes(list.sort((a, b) => a.name.localeCompare(b.name))),
      )
      .catch((e: Error) => toast.error(e.message));
  }, []);

  const loadItems = useCallback(() => {
    if (!contentType) return;
    setLoading(true);
    api
      .listContent(contentType)
      .then(setItems)
      .catch((e: Error) => toast.error(e.message))
      .finally(() => setLoading(false));
  }, [contentType]);

  useEffect(() => {
    setItems(null);
    if (contentType) loadItems();
  }, [contentType, loadItems]);

  if (types && types.length === 0) {
    return (
      <Card>
        <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
          <h3 className="text-lg font-medium">No content types yet</h3>
          <p className="max-w-sm text-sm text-muted-foreground">
            Head to the Designer to create one before authoring items.
          </p>
          <Button onClick={() => navigate("/designer")}>Go to Designer</Button>
        </CardContent>
      </Card>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        {activeType && (
          <h1 className="font-mono text-lg font-semibold">
            {activeType.name}
          </h1>
        )}
        <div className="ml-auto flex gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={loadItems}
              disabled={!contentType || loading}
            >
              <RefreshCw className={loading ? "animate-spin" : ""} />
              Refresh
            </Button>
            <Button
              size="sm"
              disabled={!activeType}
              onClick={() => {
                setEditing(null);
                setDialogOpen(true);
              }}
            >
              <Plus />
              New item
            </Button>
          </div>
        </div>

        {!contentType ? (
        <EmptyHint />
      ) : !activeType ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            Unknown content type.
          </CardContent>
        </Card>
      ) : (
        <ItemsTable
          type={activeType}
          items={items}
          onView={(item) => {
            setViewing(item);
            setViewOpen(true);
          }}
          onEdit={(item) => {
            setEditing(item);
            setDialogOpen(true);
          }}
          onDelete={async (item) => {
            if (item.id == null) return;
            if (!confirm(`Delete item #${item.id}?`)) return;
            try {
              await api.deleteContent(activeType.name, item.id);
              toast.success("Item deleted");
              loadItems();
            } catch (e) {
              toast.error((e as Error).message);
            }
          }}
        />
      )}

      {activeType && (
        <>
          <ContentItemDialog
            open={dialogOpen}
            onOpenChange={setDialogOpen}
            type={activeType}
            initial={editing}
            onSaved={() => {
              loadItems();
              toast.success(editing ? "Item updated" : "Item created");
            }}
          />
          <ViewContentItemDialog
            open={viewOpen}
            onOpenChange={setViewOpen}
            type={activeType}
            item={viewing}
            onEdit={() => {
              setEditing(viewing);
              setDialogOpen(true);
            }}
          />
        </>
      )}
    </div>
  );
}

function EmptyHint() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>Pick a content type to get started</CardTitle>
        <CardDescription>
          Select one from the sidebar to list and edit its items.
        </CardDescription>
      </CardHeader>
    </Card>
  );
}

function ItemsTable({
  type,
  items,
  onView,
  onEdit,
  onDelete,
}: {
  type: ContentType;
  items: ContentItem[] | null;
  onView: (item: ContentItem) => void;
  onEdit: (item: ContentItem) => void;
  onDelete: (item: ContentItem) => void;
}) {
  const fieldNames = Object.keys(type.fields ?? {});

  if (items === null) {
    return (
      <Card>
        <CardContent className="py-10 text-center text-sm text-muted-foreground">
          Loading…
        </CardContent>
      </Card>
    );
  }

  if (items.length === 0) {
    return (
      <Card>
        <CardContent className="py-10 text-center text-sm text-muted-foreground">
          No <span className="font-mono">{type.name}</span> items yet.
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="w-16">ID</TableHead>
            {fieldNames.map((f) => (
              <TableHead key={f} className="font-mono text-xs">
                {f}
              </TableHead>
            ))}
            <TableHead className="w-32 text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {items.map((item) => (
            <TableRow
              key={String(item.id)}
              className="cursor-pointer"
              onClick={() => onView(item)}
            >
              <TableCell className="font-mono text-xs text-muted-foreground">
                {String(item.id)}
              </TableCell>
              {fieldNames.map((f) => (
                <TableCell key={f} className="max-w-[280px]">
                  <CellValue value={item[f]} fieldType={type.fields?.[f]?.type} />
                </TableCell>
              ))}
              <TableCell
                className="text-right"
                onClick={(e) => e.stopPropagation()}
              >
                <div className="flex justify-end gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => onView(item)}
                    aria-label="View"
                  >
                    <Eye />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => onEdit(item)}
                    aria-label="Edit"
                  >
                    <Pencil />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    onClick={() => onDelete(item)}
                    aria-label="Delete"
                  >
                    <Trash2 />
                  </Button>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </Card>
  );
}

function CellValue({ value, fieldType }: { value: unknown; fieldType?: string }) {
  if (value == null || value === "")
    return <span className="text-xs text-muted-foreground italic">empty</span>;
  if (typeof value === "boolean")
    return <Badge variant={value ? "default" : "secondary"}>{String(value)}</Badge>;
  if (fieldType === "json" || (typeof value === "object" && value !== null))
    return (
      <code className="text-xs text-muted-foreground">
        {JSON.stringify(value).slice(0, 80)}
      </code>
    );
  const str = String(value);
  return <span className="line-clamp-2 break-words text-sm">{str}</span>;
}
