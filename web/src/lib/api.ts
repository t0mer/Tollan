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

export const api = {
  version: () => apiGet<VersionInfo>("/api/v1/version"),
  health: () => apiGet<Health>("/health"),
};
