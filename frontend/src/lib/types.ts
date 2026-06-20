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

export interface LoginResponse {
  token: string;
  role: string;
  expires_in: number;
}

export interface Me {
  subject: string;
  roles: string[];
  dataset: string;
}

export interface DatasetMeta {
  name: string;
  author: string;
  description: string;
  created_at: string;
  modified_at: string;
  data_format_version: number;
  schema_version: number;
  tags: string[];
}

export interface OpStat {
  count: number;
  avg_ms: number;
  total_ms: number;
}

export interface DatasetMetrics {
  total_queries: number;
  // op -> contentType -> stat
  by_op: Record<string, Record<string, OpStat>>;
}

export interface DatasetInfo {
  name: string;
  data_format_version: number;
  meta: DatasetMeta;
  stats: DatasetStats;
  approx_bytes: number;
  metrics: DatasetMetrics;
}

export interface DatasetMetaPatch {
  author?: string;
  description?: string;
  tags?: string[];
  schema_version?: number;
}
