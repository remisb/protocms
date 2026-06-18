import type {
  ContentItem,
  ContentType,
  DatasetStats,
} from "./types";

const BASE = "/api";

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const res = await fetch(BASE + path, {
    headers: { "Content-Type": "application/json" },
    ...init,
  });
  if (res.status === 204) return undefined as T;
  const text = await res.text();
  const data = text ? safeParse(text) : null;
  if (!res.ok) {
    const message =
      (data && typeof data === "object" && "error" in data && typeof data.error === "string"
        ? data.error
        : null) ?? `HTTP ${res.status}`;
    throw new Error(message);
  }
  return data as T;
}

function safeParse(text: string): unknown {
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}

export interface UploadResult {
  url: string;
  name: string;
  original?: string;
}

async function uploadFile(file: File | Blob, filename?: string): Promise<UploadResult> {
  const form = new FormData();
  form.append("file", file, filename ?? (file instanceof File ? file.name : "image"));
  const res = await fetch(BASE + "/uploads", {
    method: "POST",
    body: form,
  });
  const text = await res.text();
  const data = text ? safeParse(text) : null;
  if (!res.ok) {
    const message =
      (data && typeof data === "object" && "error" in data && typeof data.error === "string"
        ? data.error
        : null) ?? `HTTP ${res.status}`;
    throw new Error(message);
  }
  return data as UploadResult;
}

export const api = {
  getStats: () => request<DatasetStats>("/stats"),
  uploadFile,

  listContentTypes: () => request<ContentType[]>("/content-types"),
  createContentType: (ct: ContentType) =>
    request<ContentType>("/content-types", {
      method: "POST",
      body: JSON.stringify(ct),
    }),

  listContent: (
    contentType: string,
    params?: { limit?: number; filters?: Record<string, string> },
  ) => {
    const search = new URLSearchParams();
    if (params?.limit) search.set("limit", String(params.limit));
    if (params?.filters) {
      for (const [k, v] of Object.entries(params.filters)) {
        if (v !== "" && v != null) search.set(k, v);
      }
    }
    const qs = search.toString();
    return request<ContentItem[]>(
      `/content/${encodeURIComponent(contentType)}${qs ? "?" + qs : ""}`,
    );
  },

  getContent: (contentType: string, id: number | string) =>
    request<ContentItem>(
      `/content/${encodeURIComponent(contentType)}/${encodeURIComponent(String(id))}`,
    ),

  createContent: (contentType: string, item: ContentItem) =>
    request<ContentItem>(`/content/${encodeURIComponent(contentType)}`, {
      method: "POST",
      body: JSON.stringify(item),
    }),

  updateContent: (contentType: string, id: number | string, item: ContentItem) =>
    request<ContentItem>(
      `/content/${encodeURIComponent(contentType)}/${encodeURIComponent(String(id))}`,
      {
        method: "PUT",
        body: JSON.stringify(item),
      },
    ),

  deleteContent: (contentType: string, id: number | string) =>
    request<void>(
      `/content/${encodeURIComponent(contentType)}/${encodeURIComponent(String(id))}`,
      { method: "DELETE" },
    ),
};
