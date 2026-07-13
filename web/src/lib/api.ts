/** Typed fetch client for the Tollan REST API. The UI consumes only the public
 * API surface documented in api/openapi.yaml. */

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export async function apiGet<T>(path: string): Promise<T> {
  const res = await fetch(path, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  }
  return (await res.json()) as T;
}

export type VersionInfo = {
  version: string;
  commit: string;
  date: string;
  go: string;
};

export type Health = {
  status: string;
  version: string;
};

export type LogMessage = {
  id: string;
  timestamp: string;
  received_at: string;
  source: string;
  stream: string;
  input_id: string;
  message: string;
  fields?: Record<string, unknown>;
};

export type SearchResult = {
  total: number;
  count: number;
  messages: LogMessage[];
};

export type SearchParams = {
  q?: string;
  from?: string;
  to?: string;
  stream?: string;
  limit?: number;
  offset?: number;
  order?: "asc" | "desc";
  sample?: number;
  top?: number;
};

export type InputStatus = {
  id: string;
  type: string;
  bind: string;
  protocol: string;
  running: boolean;
};

export type HistogramBucket = { start_ms: number; count: number };
export type Histogram = { interval_ms: number; buckets: HistogramBucket[] };

export type FacetValue = { value: string; count: number };
export type FieldFacet = { field: string; values: FacetValue[] };

export type SavedSearch = {
  id: string;
  name: string;
  query: string;
  time_range: string;
  created_at: string;
  updated_at: string;
};

async function apiSend<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: { "Content-Type": "application/json", Accept: "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw new ApiError(res.status, `${res.status} ${res.statusText}`);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function toQS(p: Record<string, unknown>): string {
  const qs = new URLSearchParams();
  for (const [k, v] of Object.entries(p)) {
    if (v !== undefined && v !== "" && v !== null) qs.set(k, String(v));
  }
  return qs.toString();
}

export type MatchRule = {
  field: string;
  type: "exact" | "regex" | "presence" | "gt" | "lt" | "contains";
  value: string;
  negate?: boolean;
};

export type Stream = {
  id?: string;
  name: string;
  description?: string;
  combinator: "and" | "or";
  rules: MatchRule[];
  retention_days?: number;
};

export type PipelineRule = { name: string; when: string; then: string[] };
export type Pipeline = {
  id?: string;
  name: string;
  enabled: boolean;
  stages: string[];
  rules: PipelineRule[];
};

export type LookupConfig = {
  id?: string;
  name: string;
  source_type: "file" | "url";
  source: string;
  key_column: string;
  value_column: string;
};

function crud<T extends { id?: string }>(kind: string) {
  const base = `/api/v1/${kind}`;
  return {
    list: () => apiGet<T[]>(base),
    create: (b: T) => apiSend<T>("POST", base, b),
    update: (id: string, b: T) => apiSend<T>("PUT", `${base}/${id}`, b),
    remove: (id: string) => apiSend<void>("DELETE", `${base}/${id}`),
  };
}

export const streams = crud<Stream>("streams");
export const pipelines = crud<Pipeline>("pipelines");
export const lookups = crud<LookupConfig>("lookups");

export const api = {
  version: () => apiGet<VersionInfo>("/api/v1/version"),
  health: () => apiGet<Health>("/health"),
  inputs: () => apiGet<InputStatus[]>("/api/v1/inputs"),
  search: (p: SearchParams) => apiGet<SearchResult>(`/api/v1/search?${toQS(p)}`),
  histogram: (p: SearchParams) =>
    apiGet<Histogram>(`/api/v1/search/histogram?${toQS(p)}`),
  fields: (p: SearchParams) => apiGet<FieldFacet[]>(`/api/v1/search/fields?${toQS(p)}`),
  savedSearches: () => apiGet<SavedSearch[]>("/api/v1/saved-searches"),
  createSavedSearch: (b: { name: string; query: string; time_range: string }) =>
    apiSend<SavedSearch>("POST", "/api/v1/saved-searches", b),
  deleteSavedSearch: (id: string) =>
    apiSend<void>("DELETE", `/api/v1/saved-searches/${id}`),
};
