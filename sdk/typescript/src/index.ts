export const SDK_VERSION = "0.1.0";
export const APP_API_VERSION = "1";
export type JSONValue = null | boolean | number | string | JSONValue[] | {
  [key: string]: JSONValue;
};
export interface RequestOptions {
  signal?: AbortSignal;
}
export interface Identity {
  id: string;
  email: string;
}
export interface AppInfo {
  slug: string;
}
export interface CurrentUser {
  identity: Identity;
  app: AppInfo;
}
export interface Capability {
  name: string;
  version: number;
  limits?: Record<string, number>;
}
export interface KVEntry<T extends JSONValue = JSONValue> {
  key: string;
  value: T;
  version: number;
  updated_at: string;
}
export interface KVList<T extends JSONValue = JSONValue> {
  entries: KVEntry<T>[];
  next_cursor?: string;
}
export interface SetOptions extends RequestOptions {
  expectedVersion?: number;
}
export interface ListOptions extends RequestOptions {
  prefix?: string;
  limit?: number;
  cursor?: string;
}
/** Metadata for one opaque, app-scoped attachment. */
export interface TinyBlob {
  id: string;
  name: string;
  size: number;
  contentType: string;
  createdAt: string;
}
export interface BlobListOptions extends RequestOptions {
  limit?: number;
  cursor?: string;
}
export interface BlobList {
  blobs: TinyBlob[];
  nextCursor?: string;
}
export class TinyError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = new.target.name;
  }
}
export class TinyNotAuthenticatedError extends TinyError {}
export class TinyNotAuthorizedError extends TinyError {}
export class TinyCapabilityUnavailableError extends TinyError {}
export class TinyValidationError extends TinyError {}
export class TinyVersionConflictError extends TinyError {}
export class TinyQuotaExceededError extends TinyError {}
export class TinyRateLimitedError extends TinyError {}
export class TinyTemporarilyUnavailableError extends TinyError {}
/** The installed SDK major cannot safely speak to this TinyHost API. Upgrade it. */
export class TinyVersionIncompatibleError extends TinyError {}
type FetchLike = typeof fetch;
function errorFor(code: string, message: string, id?: string): TinyError {
  const C: Record<string, typeof TinyError> = {
    not_authenticated: TinyNotAuthenticatedError,
    not_authorized: TinyNotAuthorizedError,
    capability_unavailable: TinyCapabilityUnavailableError,
    validation_failed: TinyValidationError,
    version_conflict: TinyVersionConflictError,
    quota_exceeded: TinyQuotaExceededError,
    rate_limited: TinyRateLimitedError,
    temporarily_unavailable: TinyTemporarilyUnavailableError,
    sdk_version_incompatible: TinyVersionIncompatibleError,
  };
  return new (C[code] ?? TinyError)(message, code, id);
}
async function responseFor(
  fetcher: FetchLike,
  path: string,
  init: RequestInit = {},
): Promise<Response> {
  let response: Response;
  try {
    response = await fetcher(path, {
      ...init,
      credentials: "same-origin",
      headers: {
        "Accept": "application/json",
        "X-Tiny-SDK-Version": SDK_VERSION,
        "X-Tiny-App-API-Version": APP_API_VERSION,
        ...init.headers,
      },
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw new TinyTemporarilyUnavailableError(
      "TinyHost is temporarily unavailable.",
      "temporarily_unavailable",
    );
  }
  return response;
}
async function errorForResponse(response: Response): Promise<TinyError> {
  const body = await response.json().catch(() => undefined) as {
    error?: { code: string; message: string; request_id?: string };
  };
  const e = body?.error;
  return errorFor(
    e?.code ?? "temporarily_unavailable",
    e?.message ?? "TinyHost is temporarily unavailable.",
    e?.request_id,
  );
}
async function json<T>(
  fetcher: FetchLike,
  path: string,
  init: RequestInit = {},
): Promise<T> {
  const response = await responseFor(fetcher, path, init);
  if (!response.ok) throw await errorForResponse(response);
  const body = await response.json().catch(() => undefined) as T | undefined;
  if (body === undefined) {
    throw new TinyTemporarilyUnavailableError(
      "TinyHost returned an invalid response.",
      "temporarily_unavailable",
    );
  }
  return body as T;
}
async function nullableJSON<T>(
  fetcher: FetchLike,
  path: string,
  init: RequestInit = {},
): Promise<T | null> {
  try {
    return await json<T>(fetcher, path, init);
  } catch (error) {
    // A missing key is ordinary recovery state, not an exceptional transport
    // failure. Authorization and capability errors remain typed failures.
    if (error instanceof TinyError && error.code === "not_found") return null;
    throw error;
  }
}
function blobMetadata(value: unknown): TinyBlob {
  const blob = value as {
    id?: unknown;
    name?: unknown;
    size?: unknown;
    content_type?: unknown;
    created_at?: unknown;
  };
  if (
    !blob || typeof blob.id !== "string" || typeof blob.name !== "string" ||
    typeof blob.size !== "number" || !Number.isFinite(blob.size) || blob.size < 0 ||
    typeof blob.content_type !== "string" || typeof blob.created_at !== "string"
  ) {
    throw new TinyTemporarilyUnavailableError(
      "TinyHost returned invalid blob metadata.",
      "temporarily_unavailable",
    );
  }
  return {
    id: blob.id,
    name: blob.name,
    size: blob.size,
    contentType: blob.content_type,
    createdAt: blob.created_at,
  };
}
export type LiveStatus =
  | "offline"
  | "connecting"
  | "connected"
  | "reconnecting"
  | "closed";
type Handler = (event: { event: string; payload: JSONValue }) => void;
type Socket = {
  readyState: number;
  send(data: string): void;
  close(): void;
  onopen: ((e: Event) => void) | null;
  onclose: ((e: CloseEvent) => void) | null;
  onerror: ((e: Event) => void) | null;
  onmessage: ((e: MessageEvent) => void) | null;
};
export type LiveOptions = {
  webSocket: (url: string, protocols?: string | string[]) => Socket;
  origin: string;
  schedule?: (fn: () => void, ms: number) => unknown;
  cancel?: (id: unknown) => void;
  random?: () => number;
};
export interface TinyClientOptions {
  fetch?: FetchLike;
  webSocket?: (url: string, protocols?: string | string[]) => Socket;
  origin?: string;
  schedule?: LiveOptions["schedule"];
  cancel?: LiveOptions["cancel"];
  random?: () => number;
}
export interface TinyClient {
  user: {
    current(options?: RequestOptions): Promise<CurrentUser>;
  };
  app: {
    info(options?: RequestOptions): Promise<AppInfo>;
  };
  capabilities: {
    list(options?: RequestOptions): Promise<{ capabilities: Capability[] }>;
  };
  kv: {
    get<T extends JSONValue = JSONValue>(
      key: string,
      options?: RequestOptions,
    ): Promise<KVEntry<T> | null>;
    set<T extends JSONValue>(
      key: string,
      value: T,
      options?: SetOptions,
    ): Promise<KVEntry<T>>;
    delete(
      key: string,
      options?: SetOptions,
    ): Promise<{ deleted: boolean }>;
    list<T extends JSONValue = JSONValue>(
      options?: ListOptions,
    ): Promise<KVList<T>>;
  };
  blobs: {
    upload(file: File, options?: RequestOptions): Promise<TinyBlob>;
    get(id: string, options?: RequestOptions): Promise<Blob | null>;
    list(options?: BlobListOptions): Promise<BlobList>;
    delete(id: string, options?: RequestOptions): Promise<{ deleted: boolean }>;
  };
  live: {
    channel(name: string): LiveChannel;
    onKvChange(
      options: { prefix: string },
      handler: (
        event: { key: string; version: number; deleted: boolean },
      ) => void,
    ): () => void;
  };
}
export class LiveChannel {
  private handlers = new Set<Handler>();
  private socket?: Socket;
  private status: LiveStatus = "offline";
  private retry?: unknown;
  private pending?: { resolve: () => void; reject: (error: Error) => void; promise: Promise<void> };
  private wanted = false;
  private kvHandlers = new Set<
    (e: { key: string; version: number; deleted: boolean }) => void
  >();
  private kvPrefixes = new Set<string>();
  private subscribed = false;
  constructor(
    private readonly name: string | undefined,
    private readonly options: LiveOptions,
  ) {}
  on(event: string, handler: Handler): () => void {
    const wrapped: Handler = (data) => {
      if (data.event === event) handler(data);
    };
    this.handlers.add(wrapped);
    return () => {
      this.handlers.delete(wrapped);
    };
  }
  /** Subscribe this connection to its one app-scoped custom channel. */
  subscribe(): void {
    if (!this.name) return;
    this.subscribed = true;
    if (this.socket?.readyState === 1) {
      this.socket.send(JSON.stringify({ v: 1, type: "subscribe", channel: this.name }));
    }
  }
  /** Stop server-side delivery for this channel without closing the socket. */
  unsubscribe(): void {
    if (!this.name) return;
    const wasSubscribed = this.subscribed;
    this.subscribed = false;
    if (wasSubscribed && this.socket?.readyState === 1) {
      this.socket.send(JSON.stringify({ v: 1, type: "unsubscribe", channel: this.name }));
    }
  }
  onKv(
    prefix: string,
    h: (e: { key: string; version: number; deleted: boolean }) => void,
  ): () => void {
    const w = (e: { key: string; version: number; deleted: boolean }) => {
      if (e.key.startsWith(prefix)) h(e);
    };
    this.kvHandlers.add(w);
    const isNewPrefix = !this.kvPrefixes.has(prefix);
    this.kvPrefixes.add(prefix);
    if (isNewPrefix && this.socket?.readyState === 1) {
      this.subscribeKV(prefix);
    }
    return () => {
      this.kvHandlers.delete(w);
    };
  }
  connect(): Promise<void> {
    this.wanted = true;
    if (this.socket?.readyState === 1) return Promise.resolve();
    if (this.pending) return this.pending.promise;
    let resolve!: () => void;
    let reject!: (error: Error) => void;
    const promise = new Promise<void>((ok, fail) => { resolve = ok; reject = fail; });
    this.pending = { resolve, reject, promise };
    this.open();
    return promise;
  }
  private open() {
    if (!this.wanted || this.socket) return;
    this.status = this.status === "offline" ? "connecting" : "reconnecting";
    const s = this.socket = this.options.webSocket(
      this.options.origin.replace(/^http/, "ws") + "/_tiny/ws/v1",
      `tiny.sdk.${SDK_VERSION}.api.${APP_API_VERSION}`,
    );
    s.onopen = () => {
      this.status = "connected";
      if (this.name && this.subscribed) {
        s.send(JSON.stringify({ v: 1, type: "subscribe", channel: this.name }));
      }
      for (const prefix of this.kvPrefixes) {
        this.subscribeKV(prefix);
      }
      this.pending?.resolve();
      this.pending = undefined;
    };
    s.onclose = () => {
      this.socket = undefined;
      if (this.pending) {
        this.pending.reject(new TinyTemporarilyUnavailableError("Live connection closed.", "temporarily_unavailable"));
        this.pending = undefined;
      }
      if (!this.wanted) return;
      this.status = "reconnecting";
      this.retry = (this.options.schedule ?? setTimeout)(() => {
        this.retry = undefined;
        this.open();
      }, 250 + Math.floor((this.options.random ?? Math.random)() * 250));
    };
    s.onerror = () => {
      if (this.pending) {
        this.pending.reject(new TinyTemporarilyUnavailableError("Live connection failed.", "temporarily_unavailable"));
        this.pending = undefined;
      }
    };
    s.onmessage = (e) => this.receive(e.data);
  }
  private subscribeKV(prefix: string) {
    this.socket?.send(JSON.stringify({ v: 1, type: "subscribe_kv", prefix }));
  }
  private receive(raw: unknown) {
    if (typeof raw !== "string" || raw.length > 65536) return;
    try {
      const d = JSON.parse(raw) as any;
      if (d?.v !== 1) return;
      if (
        this.name && d.type === "event" && d.channel === this.name &&
        typeof d.event === "string"
      ) this.handlers.forEach((h) => h({ event: d.event, payload: d.payload }));
      if (
        d.type === "kv.changed" && typeof d.key === "string"
      ) {
        this.kvHandlers.forEach((h) =>
          h({
            key: d.key,
            version: Number(d.version) || 0,
            deleted: !!d.deleted,
          })
        );
      }
    } catch {}
  }
  publish(event: string, payload: JSONValue): void {
    if (this.socket?.readyState !== 1) {
      throw new TinyTemporarilyUnavailableError(
        "Live channel is not connected.",
        "temporarily_unavailable",
      );
    }
    this.socket.send(
      JSON.stringify({
        v: 1,
        type: "publish",
        channel: this.name,
        event,
        payload,
      }),
    );
  }
  close(): void {
    this.wanted = false;
    this.status = "closed";
    if (this.retry !== undefined) {
      (this.options.cancel ?? ((id: unknown) => clearTimeout(id as number)))(
        this.retry,
      );
    }
    this.retry = undefined;
    if (this.pending) {
      this.pending.reject(new TinyTemporarilyUnavailableError("Live connection closed.", "temporarily_unavailable"));
      this.pending = undefined;
    }
    this.socket?.close();
    this.socket = undefined;
  }
  get connectionStatus(): LiveStatus {
    return this.status;
  }
}
export function createTiny(
  options: TinyClientOptions = {},
): TinyClient {
  const fetcher = options.fetch ?? fetch;
  const liveOptions: LiveOptions = {
    webSocket: options.webSocket ?? ((url, protocols) => new WebSocket(url, protocols)),
    origin: options.origin ?? globalThis.location?.origin ?? "",
    schedule: options.schedule,
    cancel: options.cancel,
    random: options.random,
  };
  return {
    user: {
      current: (o: RequestOptions = {}) =>
        json<CurrentUser>(fetcher, "/_tiny/api/v1/me", { signal: o.signal }),
    },
    app: {
      info: (o: RequestOptions = {}) =>
        json<AppInfo>(fetcher, "/_tiny/api/v1/app", { signal: o.signal }),
    },
    capabilities: {
      list: (o: RequestOptions = {}) =>
        json<{ capabilities: Capability[] }>(
          fetcher,
          "/_tiny/api/v1/capabilities",
          { signal: o.signal },
        ),
    },
    kv: {
      get: <T extends JSONValue>(key: string, o: RequestOptions = {}) =>
        nullableJSON<KVEntry<T>>(
          fetcher,
          `/_tiny/api/v1/kv/${encodeURIComponent(key)}`,
          { signal: o.signal },
        ),
      set: <T extends JSONValue>(key: string, value: T, o: SetOptions = {}) =>
        json<KVEntry<T>>(
          fetcher,
          `/_tiny/api/v1/kv/${encodeURIComponent(key)}`,
          {
            method: "PUT",
            signal: o.signal,
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              value,
              expected_version: o.expectedVersion,
            }),
          },
        ),
      delete: (key: string, o: SetOptions = {}) =>
        json<{ deleted: boolean }>(
          fetcher,
          `/_tiny/api/v1/kv/${encodeURIComponent(key)}`,
          {
            method: "DELETE",
            signal: o.signal,
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ expected_version: o.expectedVersion }),
          },
        ),
      list: async <T extends JSONValue>(o: ListOptions = {}) => {
        const q = new URLSearchParams();
        if (o.prefix) q.set("prefix", o.prefix);
        if (o.cursor) q.set("cursor", o.cursor);
        if (o.limit) q.set("limit", String(o.limit));
        const page = await json<KVList<T>>(fetcher, `/_tiny/api/v1/kv?${q}`, {
          signal: o.signal,
        });
        // Older servers serialized an empty Go slice as `entries: null`. Keep
        // the SDK contract iterable while callers upgrade their server.
        return { ...page, entries: Array.isArray(page.entries) ? page.entries : [] };
      },
    },
    blobs: {
      upload: async (file: File, o: RequestOptions = {}) => {
        const body = new FormData();
        // The display name is multipart metadata, never a server storage key.
        body.append("file", file, file.name);
        const response = await responseFor(fetcher, "/_tiny/api/v1/blobs", {
          method: "POST",
          signal: o.signal,
          body,
        });
        if (!response.ok) throw await errorForResponse(response);
        return blobMetadata(await response.json());
      },
      get: async (id: string, o: RequestOptions = {}) => {
        const response = await responseFor(
          fetcher,
          `/_tiny/api/v1/blobs/${encodeURIComponent(id)}`,
          { signal: o.signal },
        );
        if (response.status === 404) return null;
        if (!response.ok) throw await errorForResponse(response);
        return response.blob();
      },
      list: async (o: BlobListOptions = {}) => {
        const q = new URLSearchParams();
        if (o.cursor) q.set("cursor", o.cursor);
        if (o.limit) q.set("limit", String(o.limit));
        const page = await json<{ blobs?: unknown; next_cursor?: unknown }>(
          fetcher,
          `/_tiny/api/v1/blobs?${q}`,
          { signal: o.signal },
        );
        return {
          blobs: Array.isArray(page.blobs) ? page.blobs.map(blobMetadata) : [],
          nextCursor: typeof page.next_cursor === "string" ? page.next_cursor : undefined,
        };
      },
      delete: (id: string, o: RequestOptions = {}) =>
        json<{ deleted: boolean }>(
          fetcher,
          `/_tiny/api/v1/blobs/${encodeURIComponent(id)}`,
          {
            method: "DELETE",
            signal: o.signal,
          },
        ),
    },
    live: {
      channel: (name: string) => new LiveChannel(name, liveOptions),
      onKvChange: (
        { prefix }: { prefix: string },
        handler: (
          event: { key: string; version: number; deleted: boolean },
        ) => void,
      ) => {
        // KV notifications use their own protocol frame, not a custom channel.
        // `_tiny*` is intentionally reserved and would be rejected by the hub.
        const c = new LiveChannel(undefined, liveOptions);
        const off = c.onKv(prefix, handler);
        // A listener reconnects in the background. Consume the first failed
        // connection attempt so an unavailable live service does not create an
        // unhandled rejection in an otherwise usable KV-backed app.
        void c.connect().catch(() => {});
        return () => {
          off();
          c.close();
        };
      },
    },
  };
}
export const tiny: TinyClient = createTiny();
