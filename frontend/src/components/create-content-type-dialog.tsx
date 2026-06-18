import { useEffect, useState } from "react";
import { Plus, Trash2 } from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import {
  FIELD_TYPES,
  type ContentType,
  type FieldDefinition,
  type FieldType,
} from "@/lib/types";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Separator } from "@/components/ui/separator";

interface DraftField {
  key: string;
  name: string;
  def: FieldDefinition;
}

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  existingNames: string[];
  initial?: ContentType | null;
  onSaved: () => void;
}

const SLUG = /^[a-z][a-z0-9_-]*$/;

function defaultDraft(): DraftField[] {
  return [
    {
      key: crypto.randomUUID(),
      name: "title",
      def: { type: "text", required: true },
    },
  ];
}

function draftFromType(ct: ContentType): DraftField[] {
  const entries = Object.entries(ct.fields ?? {});
  if (entries.length === 0) return defaultDraft();
  return entries.map(([name, def]) => ({
    key: crypto.randomUUID(),
    name,
    def: { ...def },
  }));
}

export function CreateContentTypeDialog({
  open,
  onOpenChange,
  existingNames,
  initial,
  onSaved,
}: Props) {
  const isEdit = !!initial;
  const [name, setName] = useState("");
  const [fields, setFields] = useState<DraftField[]>(defaultDraft);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (initial) {
      setName(initial.name);
      setFields(draftFromType(initial));
    } else {
      setName("");
      setFields(defaultDraft());
    }
    setSubmitting(false);
  }, [open, initial]);

  const addField = () =>
    setFields((f) => [
      ...f,
      { key: crypto.randomUUID(), name: "", def: { type: "text" } },
    ]);

  const removeField = (key: string) =>
    setFields((f) => f.filter((x) => x.key !== key));

  const updateField = (
    key: string,
    patch: { name?: string; def?: Partial<FieldDefinition> },
  ) =>
    setFields((f) =>
      f.map((x) =>
        x.key === key
          ? {
              ...x,
              name: patch.name ?? x.name,
              def: { ...x.def, ...(patch.def ?? {}) },
            }
          : x,
      ),
    );

  const validate = (): string | null => {
    if (!SLUG.test(name))
      return "Name must be lowercase, start with a letter, no spaces.";
    if (!isEdit && existingNames.includes(name))
      return `Type "${name}" already exists.`;
    if (fields.length === 0) return "Add at least one field.";
    const seen = new Set<string>();
    for (const f of fields) {
      if (!f.name) return "All fields need a name.";
      if (!SLUG.test(f.name))
        return `Field "${f.name}" must be lowercase, start with a letter.`;
      if (seen.has(f.name)) return `Duplicate field name "${f.name}".`;
      seen.add(f.name);
      if (f.def.type === "select" && (!f.def.options || f.def.options.length === 0))
        return `Field "${f.name}" needs at least one option.`;
      if (f.def.type === "reference" && !f.def.refType)
        return `Field "${f.name}" needs a referenced type name.`;
    }
    return null;
  };

  const submit = async () => {
    const err = validate();
    if (err) {
      toast.error(err);
      return;
    }
    setSubmitting(true);
    const payload: ContentType = {
      name,
      fields: Object.fromEntries(
        fields.map((f) => [f.name, cleanDef(f.def)]),
      ),
    };
    try {
      await api.createContentType(payload);
      onSaved();
      onOpenChange(false);
    } catch (e) {
      toast.error((e as Error).message);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>
            {isEdit ? (
              <>
                Edit content type{" "}
                <span className="font-mono">{initial!.name}</span>
              </>
            ) : (
              "New content type"
            )}
          </DialogTitle>
          <DialogDescription>
            {isEdit
              ? "Schema changes overwrite the existing type. Existing items keep their stored values, but new items will be validated against the updated schema."
              : "Define the name and fields. The schema is enforced on item creation."}
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-4">
          <div className="space-y-1.5">
            <Label htmlFor="ct-name">Name</Label>
            <Input
              id="ct-name"
              placeholder="e.g. post, dish, author"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="font-mono"
              disabled={isEdit}
            />
            {isEdit && (
              <p className="text-xs text-muted-foreground">
                Name cannot be changed.
              </p>
            )}
          </div>

          <Separator />

          <div className="space-y-3">
            <div className="flex items-center justify-between">
              <Label>Fields</Label>
              <Button
                size="sm"
                variant="outline"
                type="button"
                onClick={addField}
              >
                <Plus />
                Add field
              </Button>
            </div>

            {fields.map((f) => (
              <FieldRow
                key={f.key}
                field={f}
                onChange={(patch) => updateField(f.key, patch)}
                onRemove={() => removeField(f.key)}
                canRemove={fields.length > 1}
              />
            ))}
          </div>
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
            {submitting
              ? isEdit
                ? "Saving…"
                : "Creating…"
              : isEdit
                ? "Save changes"
                : "Create"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function FieldRow({
  field,
  onChange,
  onRemove,
  canRemove,
}: {
  field: DraftField;
  onChange: (patch: { name?: string; def?: Partial<FieldDefinition> }) => void;
  onRemove: () => void;
  canRemove: boolean;
}) {
  return (
    <div className="rounded-md border bg-muted/30 p-3 space-y-3">
      <div className="grid gap-2 sm:grid-cols-[1fr_180px_auto]">
        <Input
          placeholder="field_name"
          value={field.name}
          onChange={(e) => onChange({ name: e.target.value })}
          className="font-mono"
        />
        <Select
          value={field.def.type}
          onValueChange={(v) =>
            onChange({ def: { type: v as FieldType } })
          }
        >
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {FIELD_TYPES.map((t) => (
              <SelectItem key={t} value={t}>
                {t}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <Button
          type="button"
          variant="ghost"
          size="icon"
          onClick={onRemove}
          disabled={!canRemove}
          aria-label="Remove field"
        >
          <Trash2 />
        </Button>
      </div>

      <div className="flex items-center gap-2 text-sm">
        <Switch
          id={`${field.key}-required`}
          checked={!!field.def.required}
          onCheckedChange={(c) => onChange({ def: { required: c } })}
        />
        <Label htmlFor={`${field.key}-required`}>Required</Label>
      </div>

      {field.def.type === "select" && (
        <div className="space-y-1">
          <Label className="text-xs">Options (comma-separated)</Label>
          <Input
            placeholder="tech, news, tutorial"
            value={(field.def.options ?? []).join(", ")}
            onChange={(e) =>
              onChange({
                def: {
                  options: e.target.value
                    .split(",")
                    .map((s) => s.trim())
                    .filter(Boolean),
                },
              })
            }
          />
        </div>
      )}

      {field.def.type === "reference" && (
        <div className="space-y-1">
          <Label className="text-xs">Referenced type</Label>
          <Input
            placeholder="author"
            value={field.def.refType ?? ""}
            onChange={(e) => onChange({ def: { refType: e.target.value } })}
            className="font-mono"
          />
        </div>
      )}
    </div>
  );
}

function cleanDef(def: FieldDefinition): FieldDefinition {
  const out: FieldDefinition = { type: def.type };
  if (def.required) out.required = true;
  if (def.type === "select" && def.options?.length) out.options = def.options;
  if (def.type === "reference" && def.refType) out.refType = def.refType;
  if (def.default !== undefined) out.default = def.default;
  return out;
}
