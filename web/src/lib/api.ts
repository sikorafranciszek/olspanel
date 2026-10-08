export class ApiError extends Error {
  code: string;
  status: number;
  constructor(message: string, code: string, status: number) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

let csrfToken = "";

export function setCsrf(token: string) {
  csrfToken = token;
}

export function getCsrf() {
  return csrfToken;
}

async function parseError(res: Response): Promise<ApiError> {
  let msg = `Błąd ${res.status}`;
  let code = "error";
  try {
    const j = await res.json();
    if (j && typeof j.error === "string") msg = j.error;
    if (j && typeof j.code === "string") code = j.code;
  } catch {
    /* ignore */
  }
  return new ApiError(msg, code, res.status);
}

export async function api<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
  let payload: BodyInit | undefined;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  const res = await fetch(`/api/v1${path}`, { method, headers, body: payload, credentials: "same-origin" });
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const get = <T>(path: string) => api<T>("GET", path);
export const post = <T>(path: string, body?: unknown) => api<T>("POST", path, body ?? {});
export const put = <T>(path: string, body?: unknown) => api<T>("PUT", path, body ?? {});
export const del = <T>(path: string) => api<T>("DELETE", path);

export async function uploadFiles(dir: string, files: FileList | File[]): Promise<{ uploaded: number }> {
  const fd = new FormData();
  fd.append("path", dir);
  for (const f of Array.from(files)) fd.append("files", f, f.name);
  const headers: Record<string, string> = {};
  if (csrfToken) headers["X-CSRF-Token"] = csrfToken;
  const res = await fetch(`/api/v1/files/upload?path=${encodeURIComponent(dir)}`, { method: "POST", headers, body: fd, credentials: "same-origin" });
  if (!res.ok) throw await parseError(res);
  return res.json();
}

export function downloadUrl(path: string) {
  return `/api/v1/files/download?path=${encodeURIComponent(path)}`;
}

export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return "Nieznany błąd";
}
