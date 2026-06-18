export const FIELD_TYPES = [
  "text",
  "textarea",
  "richText",
  "number",
  "boolean",
  "date",
  "datetime",
  "image",
  "media",
  "select",
  "reference",
  "slug",
  "json",
] as const;

export type FieldType = (typeof FIELD_TYPES)[number];

export interface FieldDefinition {
  type: FieldType;
  required?: boolean;
  options?: string[];
  refType?: string;
  default?: unknown;
}

export interface ContentType {
  name: string;
  fields?: Record<string, FieldDefinition>;
}

export type ContentItem = { id?: number } & Record<string, unknown>;

export interface DatasetStats {
  dataset: string;
  content_types: number;
  total_items: number;
  items_per_type: Record<string, number>;
}
