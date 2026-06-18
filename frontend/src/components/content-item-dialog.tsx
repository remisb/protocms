import { useEffect, useState } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import type { ContentItem, ContentType, FieldDefinition } from "@/lib/types";
import { Button } from "@/components/ui/button";
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
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ImageField } from "@/components/image-field";

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  type: ContentType;
  initial: ContentItem | null;
  onSaved: () => void;
}

export function ContentItemDialog({
  open,
  onOpenChange,
  type,
  initial,
  onSaved,
}: Props) {
  const [values, setValues] = useState<Record<string, unknown>>({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (initial) {
      const { id: _id, ...rest } = initial;
      void _id;
      setValues(rest);
    } else {
      const defaults: Record<string, unknown> = {};
      for (const [name, def] of Object.entries(type.fields ?? {})) {
        if (def.default !== undefined) defaults[name] = def.default;
      }
      setValues(defaults);
    }
  }, [open, initial, type]);

  const setField = (name: string, value: unknown) =>
    setValues((v) => ({ ...v, [name]: value }));

  const submit = async () => {
    const payload = preparePayload(values, type);
    setSubmitting(true);
    try {
      if (initial?.id != null) {
        await api.updateContent(type.name, initial.id, payload);
      } else {
        await api.createContent(type.name, payload);
      }
      onSaved();
      onOpenChange(false);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  const fields = Object.entries(type.fields ?? {});

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {initial ? "Edit" : "New"} <span className="font-mono">{type.name}</span>
            {initial?.id != null && (
              <span className="ml-2 font-mono text-sm text-muted-foreground">
                #{String(initial.id)}
              </span>
            )}
          </DialogTitle>
          <DialogDescription>
            Fields below come from this content type's schema.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          {fields.length === 0 && (
            <p className="text-sm text-muted-foreground">
              This content type has no fields defined.
            </p>
          )}
          {fields.map(([name, def]) => (
            <FieldInput
              key={name}
              name={name}
              def={def}
              value={values[name]}
              onChange={(v) => setField(name, v)}
            />
          ))}
        </div>

        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            Cancel
          </Button>
          <Button onClick={submit} disabled={submitting}>
            {submitting ? "Saving…" : initial ? "Save changes" : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function FieldInput({
  name,
  def,
  value,
  onChange,
}: {
  name: string;
  def: FieldDefinition;
  value: unknown;
  onChange: (v: unknown) => void;
}) {
  const id = `f-${name}`;
  const labelEl = (
    <div className="flex items-center gap-2">
      <Label htmlFor={id} className="font-mono text-xs">
        {name}
      </Label>
      <span className="text-[10px] text-muted-foreground">{def.type}</span>
      {def.required && (
        <span className="text-[10px] font-medium text-destructive">required</span>
      )}
    </div>
  );

  switch (def.type) {
    case "textarea":
    case "richText":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Textarea
            id={id}
            rows={def.type === "richText" ? 6 : 3}
            value={value == null ? "" : String(value)}
            onChange={(e) => onChange(e.target.value)}
          />
        </div>
      );

    case "number":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Input
            id={id}
            type="number"
            value={value == null ? "" : String(value)}
            onChange={(e) => {
              const v = e.target.value;
              onChange(v === "" ? undefined : Number(v));
            }}
          />
        </div>
      );

    case "boolean":
      return (
        <div className="flex items-center justify-between rounded-md border p-3">
          {labelEl}
          <Switch
            id={id}
            checked={value === true}
            onCheckedChange={onChange}
          />
        </div>
      );

    case "date":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Input
            id={id}
            type="date"
            value={value == null ? "" : String(value).slice(0, 10)}
            onChange={(e) => onChange(e.target.value)}
          />
        </div>
      );

    case "datetime":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Input
            id={id}
            type="datetime-local"
            value={value == null ? "" : String(value).slice(0, 16)}
            onChange={(e) =>
              onChange(e.target.value ? `${e.target.value}:00Z` : "")
            }
          />
        </div>
      );

    case "select":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Select
            value={value == null ? "" : String(value)}
            onValueChange={onChange}
          >
            <SelectTrigger id={id}>
              <SelectValue placeholder="Choose…" />
            </SelectTrigger>
            <SelectContent>
              {(def.options ?? []).map((opt) => (
                <SelectItem key={opt} value={opt}>
                  {opt}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      );

    case "reference":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Input
            id={id}
            placeholder={`ID of a ${def.refType ?? "referenced"} item`}
            value={value == null ? "" : String(value)}
            onChange={(e) => onChange(e.target.value)}
          />
        </div>
      );

    case "json":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Textarea
            id={id}
            rows={4}
            className="font-mono text-xs"
            value={value == null ? "" : JSON.stringify(value, null, 2)}
            onChange={(e) => {
              try {
                onChange(JSON.parse(e.target.value));
              } catch {
                onChange(e.target.value);
              }
            }}
          />
        </div>
      );

    case "image":
      return (
        <div className="space-y-1.5">
          {labelEl}
          <ImageField
            id={id}
            value={value == null ? "" : String(value)}
            onChange={onChange}
          />
        </div>
      );

    case "media":
    case "text":
    case "slug":
    default:
      return (
        <div className="space-y-1.5">
          {labelEl}
          <Input
            id={id}
            value={value == null ? "" : String(value)}
            onChange={(e) => onChange(e.target.value)}
          />
        </div>
      );
  }
}

function preparePayload(
  values: Record<string, unknown>,
  type: ContentType,
): ContentItem {
  const out: ContentItem = {};
  for (const [name, def] of Object.entries(type.fields ?? {})) {
    const v = values[name];
    if (v === undefined || v === "") {
      if (def.required) out[name] = v;
      continue;
    }
    out[name] = v;
  }
  return out;
}
