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
};

export type InputStatus = {
  id: string;
  type: string;
  bind: string;
  protocol: string;
  running: boolean;
};

export const api = {
  version: () => apiGet<VersionInfo>("/api/v1/version"),
  health: () => apiGet<Health>("/health"),
  inputs: () => apiGet<InputStatus[]>("/api/v1/inputs"),
  search: (p: SearchParams) => {
    const qs = new URLSearchParams();
    for (const [k, v] of Object.entries(p)) {
      if (v !== undefined && v !== "" && v !== null) qs.set(k, String(v));
    }
    return apiGet<SearchResult>(`/api/v1/search?${qs.toString()}`);
  },
};
