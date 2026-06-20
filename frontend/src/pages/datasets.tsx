import { useCallback, useEffect, useState } from "react";
import { Database, Plus, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { DatasetInfo, DatasetMetaPatch } from "@/lib/types";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / 1024 / 1024).toFixed(1)} MB`;
}

export function DatasetsPage() {
  const [datasets, setDatasets] = useState<DatasetInfo[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [loadName, setLoadName] = useState("");
  const [detail, setDetail] = useState<DatasetInfo | null>(null);

  const load = useCallback(() => {
    setLoading(true);
    api
      .listDatasets()
      .then((list) =>
        setDatasets(list.sort((a, b) => a.name.localeCompare(b.name))),
      )
      .catch((e: Error) => toast.error(e.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => load(), [load]);

  const onLoad = async () => {
    const name = loadName.trim();
    if (!name) return;
    try {
      await api.loadDataset(name);
      toast.success(`Loaded ${name}`);
      setLoadName("");
      load();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  const onUnload = async (name: string) => {
    if (!confirm(`Unload ${name}? (files are kept on disk)`)) return;
    try {
      await api.unloadDataset(name);
      toast.success(`Unloaded ${name}`);
      load();
    } catch (e) {
      toast.error((e as Error).message);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Datasets</h1>
          <p className="text-sm text-muted-foreground">
            Datasets currently loaded in memory, with size and query metrics.
          </p>
        </div>
        <div className="flex items-end gap-2">
          <div className="space-y-1.5">
            <Label htmlFor="load-name" className="text-xs">
              Load dataset
            </Label>
            <Input
              id="load-name"
              value={loadName}
              onChange={(e) => setLoadName(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && onLoad()}
              placeholder="name"
              className="w-40"
            />
          </div>
          <Button size="sm" onClick={onLoad} disabled={!loadName.trim()}>
            <Plus />
            Load
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={load}
            disabled={loading}
          >
            <RefreshCw className={loading ? "animate-spin" : ""} />
            Refresh
          </Button>
        </div>
      </div>

      {datasets === null ? (
        <Card>
          <CardContent className="py-10 text-center text-sm text-muted-foreground">
            Loading…
          </CardContent>
        </Card>
      ) : datasets.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
            <Database className="size-6 text-muted-foreground" />
            <p className="text-sm text-muted-foreground">
              No datasets loaded.
            </p>
          </CardContent>
        </Card>
      ) : (
        <Card>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Name</TableHead>
                <TableHead>Format</TableHead>
                <TableHead className="text-right">Types</TableHead>
                <TableHead className="text-right">Items</TableHead>
                <TableHead className="text-right">Memory</TableHead>
                <TableHead className="text-right">Queries</TableHead>
                <TableHead className="w-40 text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {datasets.map((d) => (
                <TableRow key={d.name}>
                  <TableCell className="font-mono font-medium">
                    {d.name}
                  </TableCell>
                  <TableCell>
                    <Badge
                      variant={
                        d.data_format_version >= 2 ? "secondary" : "outline"
                      }
                    >
                      v{d.data_format_version}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-right">
                    {d.stats.content_types}
                  </TableCell>
                  <TableCell className="text-right">
                    {d.stats.total_items}
                  </TableCell>
                  <TableCell className="text-right">
                    {formatBytes(d.approx_bytes)}
                  </TableCell>
                  <TableCell className="text-right">
                    {d.metrics.total_queries}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => setDetail(d)}
                      >
                        Details
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        onClick={() => onUnload(d.name)}
                      >
                        Unload
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Card>
      )}

      <DatasetDetailDialog
        info={detail}
        onOpenChange={(open) => !open && setDetail(null)}
        onSaved={load}
      />
    </div>
  );
}

function DatasetDetailDialog({
  info,
  onOpenChange,
  onSaved,
}: {
  info: DatasetInfo | null;
  onOpenChange: (open: boolean) => void;
  onSaved: () => void;
}) {
  const [author, setAuthor] = useState("");
  const [description, setDescription] = useState("");
  const [tags, setTags] = useState("");
  const [schemaVersion, setSchemaVersion] = useState("1");
  const [saving, setSaving] = useState(false);

  // Seed the form whenever a new dataset is opened.
  useEffect(() => {
    if (!info) return;
    setAuthor(info.meta.author ?? "");
    setDescription(info.meta.description ?? "");
    setTags((info.meta.tags ?? []).join(", "));
    setSchemaVersion(String(info.meta.schema_version ?? 1));
  }, [info]);

  if (!info) return null;

  const isLegacy = info.data_format_version < 2;

  const save = async () => {
    const patch: DatasetMetaPatch = {
      author,
      description,
      tags: tags
        .split(",")
        .map((t) => t.trim())
        .filter(Boolean),
      schema_version: Number(schemaVersion) || 1,
    };
    setSaving(true);
    try {
      await api.updateDataset(info.name, patch);
      toast.success("Metadata saved");
      onSaved();
      onOpenChange(false);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={!!info} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[85vh] overflow-y-auto sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="font-mono">{info.name}</DialogTitle>
          <DialogDescription>
            {isLegacy
              ? "Legacy dataset — migrate it to edit metadata."
              : `Last modified ${new Date(info.meta.modified_at).toLocaleString()}`}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="ds-author">Author</Label>
            <Input
              id="ds-author"
              value={author}
              onChange={(e) => setAuthor(e.target.value)}
              disabled={isLegacy || saving}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="ds-desc">Description</Label>
            <Textarea
              id="ds-desc"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={isLegacy || saving}
            />
          </div>
          <div className="grid grid-cols-2 gap-4">
            <div className="space-y-1.5">
              <Label htmlFor="ds-tags">Tags (comma-separated)</Label>
              <Input
                id="ds-tags"
                value={tags}
                onChange={(e) => setTags(e.target.value)}
                disabled={isLegacy || saving}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ds-schema">Schema version</Label>
              <Input
                id="ds-schema"
                type="number"
                value={schemaVersion}
                onChange={(e) => setSchemaVersion(e.target.value)}
                disabled={isLegacy || saving}
              />
            </div>
          </div>

          <MetricsTable info={info} />
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={saving}
          >
            Close
          </Button>
          <Button onClick={save} disabled={isLegacy || saving}>
            {saving ? "Saving…" : "Save metadata"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function MetricsTable({ info }: { info: DatasetInfo }) {
  const rows: { op: string; ct: string; count: number; avgMs: number }[] = [];
  for (const [op, byType] of Object.entries(info.metrics.by_op)) {
    for (const [ct, stat] of Object.entries(byType)) {
      rows.push({ op, ct, count: stat.count, avgMs: stat.avg_ms });
    }
  }
  rows.sort((a, b) => b.count - a.count);

  return (
    <div className="space-y-2">
      <h3 className="text-sm font-medium">
        Query metrics
        <span className="ml-2 text-xs text-muted-foreground">
          {info.metrics.total_queries} total
        </span>
      </h3>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">No queries recorded yet.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Operation</TableHead>
              <TableHead>Content type</TableHead>
              <TableHead className="text-right">Count</TableHead>
              <TableHead className="text-right">Avg ms</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={`${r.op}:${r.ct}`}>
                <TableCell>
                  <Badge variant="secondary">{r.op}</Badge>
                </TableCell>
                <TableCell className="font-mono">{r.ct}</TableCell>
                <TableCell className="text-right">{r.count}</TableCell>
                <TableCell className="text-right">
                  {r.avgMs.toFixed(3)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
    </div>
  );
}
