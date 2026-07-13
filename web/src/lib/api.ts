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

export type WidgetType = "table" | "histogram" | "bar" | "line" | "area" | "pie" | "stat" | "topn" | "map";
export type Widget = {
  id: string;
  type: WidgetType;
  title: string;
  query?: string;
  time_range?: string;
  group_by?: string;
  metric?: string;
  metric_field?: string;
  w: number;
  h: number;
};
export type Dashboard = {
  id?: string;
  name: string;
  refresh_seconds?: number;
  time_range?: string;
  widgets: Widget[];
};

export type AggRow = { key: string; value: number };
export type AggResponse = { rows: AggRow[] };

export type AggParams = SearchParams & {
  group_by?: string;
  metric?: string;
  metric_field?: string;
};

export type EventDefinition = {
  id?: string;
  name: string;
  enabled: boolean;
  type: "filter" | "aggregation";
  query: string;
  window_seconds: number;
  threshold: number;
  group_by?: string;
  metric?: string;
  metric_field?: string;
  channels: string[];
  message_template?: string;
  grace_seconds?: number;
  backlog?: number;
};

export type FiredEvent = {
  id: string;
  definition_name: string;
  fired_at: string;
  message: string;
  count: number;
  group_key: string;
};

export type Channel = {
  id?: string;
  name: string;
  provider: "shoutrrr" | "greenapi" | "whatsapp_web";
  enabled: boolean;
  notify_on_success: boolean;
  notify_on_failure: boolean;
  url?: string;
  instance_id?: string;
  token?: string;
  phone?: string;
  api_url?: string;
  base_url?: string;
  username?: string;
  password?: string;
};

export type Output = {
  id?: string;
  name: string;
  type: "gelf" | "tcp_raw" | "tcp_syslog" | "stdout";
  enabled: boolean;
  stream?: string;
  address?: string;
  protocol?: string;
};

export type ImportChange = { kind: string; id: string; name: string; action: string };
export type ImportResult = { dry_run: boolean; changes: ImportChange[] };

export const streams = crud<Stream>("streams");
export const pipelines = crud<Pipeline>("pipelines");
export const lookups = crud<LookupConfig>("lookups");
export const dashboards = crud<Dashboard>("dashboards");
export const eventDefinitions = crud<EventDefinition>("event-definitions");
export const outputs = crud<Output>("outputs");

export const contentPacks = {
  exportUrl: () => "/api/v1/content-packs/export",
  import: (bundle: unknown, dryRun: boolean) =>
    apiSend<ImportResult>("POST", `/api/v1/content-packs/import?dry_run=${dryRun}`, bundle),
};

export const channels = {
  list: () => apiGet<Channel[]>("/api/v1/notifications"),
  create: (b: Channel) => apiSend<Channel>("POST", "/api/v1/notifications", b),
  update: (id: string, b: Channel) => apiSend<Channel>("PUT", `/api/v1/notifications/${id}`, b),
  remove: (id: string) => apiSend<void>("DELETE", `/api/v1/notifications/${id}`),
  test: (b: Channel) => apiSend<{ status: string }>("POST", "/api/v1/notifications/test", b),
};

export function exportUrl(p: SearchParams, format: "csv" | "json"): string {
  return `/api/v1/search/export?${toQS({ ...p, format })}`;
}

export const api = {
  version: () => apiGet<VersionInfo>("/api/v1/version"),
  health: () => apiGet<Health>("/health"),
  inputs: () => apiGet<InputStatus[]>("/api/v1/inputs"),
  search: (p: SearchParams) => apiGet<SearchResult>(`/api/v1/search?${toQS(p)}`),
  histogram: (p: SearchParams) =>
    apiGet<Histogram>(`/api/v1/search/histogram?${toQS(p)}`),
  fields: (p: SearchParams) => apiGet<FieldFacet[]>(`/api/v1/search/fields?${toQS(p)}`),
  aggregate: (p: AggParams) => apiGet<AggResponse>(`/api/v1/search/aggregate?${toQS(p)}`),
  events: () => apiGet<FiredEvent[]>("/api/v1/events"),
  savedSearches: () => apiGet<SavedSearch[]>("/api/v1/saved-searches"),
  createSavedSearch: (b: { name: string; query: string; time_range: string }) =>
    apiSend<SavedSearch>("POST", "/api/v1/saved-searches", b),
  deleteSavedSearch: (id: string) =>
    apiSend<void>("DELETE", `/api/v1/saved-searches/${id}`),
};
