import type { ContentItem, ContentType, FieldDefinition } from "@/lib/types";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  type: ContentType;
  item: ContentItem | null;
  onEdit?: () => void;
}

export function ViewContentItemDialog({
  open,
  onOpenChange,
  type,
  item,
  onEdit,
}: Props) {
  if (!item) return null;
  const fields = Object.entries(type.fields ?? {});
  const knownNames = new Set(fields.map(([n]) => n));
  const extraEntries = Object.entries(item).filter(
    ([name]) => name !== "id" && !knownNames.has(name),
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            <span className="font-mono">{type.name}</span>
            {item.id != null && (
              <span className="ml-2 font-mono text-sm text-muted-foreground">
                #{String(item.id)}
              </span>
            )}
          </DialogTitle>
          <DialogDescription>Read-only view of this item.</DialogDescription>
        </DialogHeader>

        <dl className="divide-y rounded-md border bg-muted/30">
          <Row label="id" value={item.id} mono />
          {fields.map(([name, def]) => (
            <Row
              key={name}
              label={name}
              hint={def.type}
              value={item[name]}
              def={def}
            />
          ))}
          {extraEntries.length > 0 && (
            <div className="px-3 py-2 text-xs text-muted-foreground">
              Extra fields
            </div>
          )}
          {extraEntries.map(([name, value]) => (
            <Row key={name} label={name} value={value} extra />
          ))}
        </dl>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Close
          </Button>
          {onEdit && (
            <Button
              onClick={() => {
                onOpenChange(false);
                onEdit();
              }}
            >
              Edit
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Row({
  label,
  hint,
  value,
  def,
  mono,
  extra,
}: {
  label: string;
  hint?: string;
  value: unknown;
  def?: FieldDefinition;
  mono?: boolean;
  extra?: boolean;
}) {
  return (
    <div className="grid grid-cols-[200px_1fr] gap-3 px-3 py-2 text-sm">
      <dt className="flex items-center gap-2">
        <span className={mono ? "font-mono text-xs" : "font-mono text-xs"}>
          {label}
        </span>
        {hint && (
          <span className="text-[10px] text-muted-foreground">{hint}</span>
        )}
        {extra && (
          <Badge variant="outline" className="text-[10px]">
            extra
          </Badge>
        )}
      </dt>
      <dd className="min-w-0 break-words">
        <Value value={value} def={def} />
      </dd>
    </div>
  );
}

function Value({ value, def }: { value: unknown; def?: FieldDefinition }) {
  if (value == null || value === "") {
    return <span className="text-xs italic text-muted-foreground">empty</span>;
  }
  if (typeof value === "boolean") {
    return (
      <Badge variant={value ? "default" : "secondary"}>{String(value)}</Badge>
    );
  }
  if (def?.type === "reference") {
    return (
      <span className="font-mono text-xs">
        {def.refType ? `${def.refType}#` : "#"}
        {String(value)}
      </span>
    );
  }
  if (def?.type === "image" || def?.type === "media") {
    const url = String(value);
    const isImage = def.type === "image" || /\.(png|jpe?g|gif|webp|svg)$/i.test(url);
    return (
      <div className="space-y-1">
        <a
          href={url}
          target="_blank"
          rel="noreferrer"
          className="text-xs text-primary underline break-all"
        >
          {url}
        </a>
        {isImage && (
          <img
            src={url}
            alt=""
            className="mt-1 max-h-48 rounded border object-contain"
          />
        )}
      </div>
    );
  }
  if (def?.type === "json" || (typeof value === "object" && value !== null)) {
    return (
      <pre className="overflow-x-auto rounded bg-background p-2 text-xs">
        {JSON.stringify(value, null, 2)}
      </pre>
    );
  }
  if (def?.type === "richText" || def?.type === "textarea") {
    return <p className="whitespace-pre-wrap text-sm">{String(value)}</p>;
  }
  return <span className="text-sm">{String(value)}</span>;
}
