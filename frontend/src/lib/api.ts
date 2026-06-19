import type {
  ContentItem,
  ContentType,
  DatasetStats,
  LoginResponse,
  Me,
} from "./types";

const BASE = "/api";
const TOKEN_KEY = "protocms_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null): void {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

function authHeaders(base?: HeadersInit): Headers {
  const headers = new Headers(base);
  const token = getToken();
  if (token) headers.set("Authorization", `Bearer ${token}`);
  return headers;
}

// Drop the session and notify the app when the server rejects our token.
function handleUnauthorized(): void {
  setToken(null);
  window.dispatchEvent(new Event("protocms:unauthorized"));
}

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const headers = authHeaders(init?.headers);
  if (!headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  const res = await fetch(BASE + path, { ...init, headers });
  if (res.status === 401) handleUnauthorized();
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
    headers: authHeaders(),
  });
  if (res.status === 401) handleUnauthorized();
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

  login: (username: string, password: string) =>
    request<LoginResponse>("/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    }),
  me: () => request<Me>("/me"),
  logout: () => setToken(null),

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
