export const SDK_VERSION = "0.1.1";
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
  /** Browser-safe notice when a capability sends app content outside Tinkercloud. */
  disclosure?: string;
}
export type ChatMessage = { role: "user" | "assistant"; content: string };
export interface ChatRequest { messages: ChatMessage[]; maxOutputTokens?: number; }
export interface ChatResponse {
  message: { role: "assistant"; content: string };
  usage: { inputTokens: number; outputTokens: number };
  finishReason: "stop" | "length";
  requestId: string;
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
export type JSONObject = { [key: string]: JSONValue };
export interface TinkerDocument<T extends JSONObject = JSONObject> {
  id: string;
  data: T;
  version: number;
  createdAt: string;
  updatedAt: string;
}
export interface CollectionList<T extends JSONObject = JSONObject> {
  documents: TinkerDocument<T>[];
  revision: number;
  nextCursor?: string;
}
export interface CollectionListOptions extends RequestOptions {
  limit?: number;
  cursor?: string;
}
export interface CollectionWriteOptions extends RequestOptions {
  expectedVersion?: number;
}
export type CollectionSyncStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "error"
  | "closed";
export interface CollectionSubscription<T extends JSONObject = JSONObject> {
  onSnapshot(snapshot: CollectionList<T>): void;
  onCreate?(document: TinkerDocument<T>): void;
  onUpdate?(document: TinkerDocument<T>): void;
  onDelete?(id: string): void;
  onStatus?(status: CollectionSyncStatus): void;
  onError?(error: unknown): void;
}
export interface TinkerCollection<T extends JSONObject = JSONObject> {
  create(data: T, options?: RequestOptions): Promise<TinkerDocument<T>>;
  get(id: string, options?: RequestOptions): Promise<TinkerDocument<T> | null>;
  update(
    id: string,
    data: T,
    options?: CollectionWriteOptions,
  ): Promise<TinkerDocument<T>>;
  delete(
    id: string,
    options?: CollectionWriteOptions,
  ): Promise<{ deleted: boolean }>;
  list(options?: CollectionListOptions): Promise<CollectionList<T>>;
  subscribe(handlers: CollectionSubscription<T>): () => void;
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
export interface TinkerBlob {
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
  blobs: TinkerBlob[];
  nextCursor?: string;
}
export class TinkerError extends Error {
  constructor(
    message: string,
    public readonly code: string,
    public readonly requestId?: string,
  ) {
    super(message);
    this.name = new.target.name;
  }
}
export class TinkerNotAuthenticatedError extends TinkerError {}
export class TinkerNotAuthorizedError extends TinkerError {}
export class TinkerCapabilityUnavailableError extends TinkerError {}
export class TinkerValidationError extends TinkerError {}
export class TinkerVersionConflictError extends TinkerError {}
export class TinkerQuotaExceededError extends TinkerError {}
export class TinkerRateLimitedError extends TinkerError {}
export class TinkerTemporarilyUnavailableError extends TinkerError {}
/** The installed SDK major cannot safely speak to this Tinkercloud API. Upgrade it. */
export class TinkerVersionIncompatibleError extends TinkerError {}
/** The chat request was rejected before provider work. */
export class TinkerInvalidRequestError extends TinkerError {}
/** The operator-approved chat budget has been exhausted. */
export class TinkerQuotaExhaustedError extends TinkerError {}
/** The chat request was cancelled. */
export class TinkerCancelledError extends TinkerError {}
type FetchLike = typeof fetch;
function errorFor(code: string, message: string, id?: string): TinkerError {
  const C: Record<string, typeof TinkerError> = {
    not_authenticated: TinkerNotAuthenticatedError,
    not_authorized: TinkerNotAuthorizedError,
    capability_unavailable: TinkerCapabilityUnavailableError,
    validation_failed: TinkerValidationError,
    version_conflict: TinkerVersionConflictError,
    quota_exceeded: TinkerQuotaExceededError,
    rate_limited: TinkerRateLimitedError,
    temporarily_unavailable: TinkerTemporarilyUnavailableError,
    sdk_version_incompatible: TinkerVersionIncompatibleError,
    unauthorized: TinkerNotAuthorizedError,
    invalid_request: TinkerInvalidRequestError,
    quota_exhausted: TinkerQuotaExhaustedError,
    cancelled: TinkerCancelledError,
  };
  return new (C[code] ?? TinkerError)(message, code, id);
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
        "X-Tinker-SDK-Version": SDK_VERSION,
        "X-Tinker-App-API-Version": APP_API_VERSION,
        ...init.headers,
      },
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw new TinkerTemporarilyUnavailableError(
      "Tinkercloud is temporarily unavailable.",
      "temporarily_unavailable",
    );
  }
  return response;
}
async function errorForResponse(response: Response): Promise<TinkerError> {
  const body = await response.json().catch(() => undefined) as {
    error?: { code: string; message: string; request_id?: string };
  };
  const e = body?.error;
  return errorFor(
    e?.code ?? "temporarily_unavailable",
    e?.message ?? "Tinkercloud is temporarily unavailable.",
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
    throw new TinkerTemporarilyUnavailableError(
      "Tinkercloud returned an invalid response.",
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
    if (error instanceof TinkerError && error.code === "not_found") return null;
    throw error;
  }
}
function blobMetadata(value: unknown): TinkerBlob {
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
    throw new TinkerTemporarilyUnavailableError(
      "Tinkercloud returned invalid blob metadata.",
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
function chatResponse(value: unknown): ChatResponse {
  const reply = value as { message?: { role?: unknown; content?: unknown }; usage?: { input_tokens?: unknown; output_tokens?: unknown }; finish_reason?: unknown; request_id?: unknown };
  const input = reply?.usage?.input_tokens;
  const output = reply?.usage?.output_tokens;
  if (
    !reply || reply.message?.role !== "assistant" || typeof reply.message.content !== "string" ||
    typeof input !== "number" || !Number.isSafeInteger(input) || input < 0 ||
    typeof output !== "number" || !Number.isSafeInteger(output) || output < 0 ||
    (reply.finish_reason !== "stop" && reply.finish_reason !== "length") ||
    typeof reply.request_id !== "string" || reply.request_id.length === 0
  ) throw new TinkerTemporarilyUnavailableError("Tinkercloud returned an invalid chat response.", "temporarily_unavailable");
  return { message: { role: "assistant", content: reply.message.content }, usage: { inputTokens: input, outputTokens: output }, finishReason: reply.finish_reason, requestId: reply.request_id };
}
function documentMetadata<T extends JSONObject>(
  value: unknown,
): TinkerDocument<T> {
  const document = value as {
    id?: unknown;
    data?: unknown;
    version?: unknown;
    created_at?: unknown;
    updated_at?: unknown;
  };
  if (
    !document || typeof document.id !== "string" ||
    !document.data || typeof document.data !== "object" ||
    Array.isArray(document.data) ||
    typeof document.version !== "number" ||
    !Number.isSafeInteger(document.version) || document.version < 1 ||
    typeof document.created_at !== "string" ||
    typeof document.updated_at !== "string"
  ) {
    throw new TinkerTemporarilyUnavailableError(
      "Tinkercloud returned invalid document metadata.",
      "temporarily_unavailable",
    );
  }
  return {
    id: document.id,
    data: document.data as T,
    version: document.version,
    createdAt: document.created_at,
    updatedAt: document.updated_at,
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
export interface TinkerClientOptions {
  fetch?: FetchLike;
  webSocket?: (url: string, protocols?: string | string[]) => Socket;
  origin?: string;
  schedule?: LiveOptions["schedule"];
  cancel?: LiveOptions["cancel"];
  random?: () => number;
}
export interface TinkerClient {
  user: {
    current(options?: RequestOptions): Promise<CurrentUser>;
  };
  app: {
    info(options?: RequestOptions): Promise<AppInfo>;
  };
  capabilities: {
    list(options?: RequestOptions): Promise<{ capabilities: Capability[] }>;
  };
  llm: { chat: { complete(request: ChatRequest, options?: RequestOptions): Promise<ChatResponse>; }; };
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
  db: {
    collection<T extends JSONObject = JSONObject>(
      name: string,
    ): TinkerCollection<T>;
  };
  blobs: {
    upload(file: File, options?: RequestOptions): Promise<TinkerBlob>;
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
  private kvHandlers = new Map<string, Set<
    (e: { key: string; version: number; deleted: boolean }) => void
  >>();
  private subscribed = false;
  private collectionHandlers = new Map<string, Set<
    (e: { collection: string; id: string; version: number; deleted: boolean; revision: number }) => void
  >>();
  private statusHandlers = new Set<(status: LiveStatus) => void>();
  constructor(
    private readonly name: string | undefined,
    private readonly options: LiveOptions,
    private readonly closeWhenIdle = false,
    private readonly onIdle?: () => void,
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
    let handlers = this.kvHandlers.get(prefix);
    const first = !handlers;
    if (!handlers) {
      handlers = new Set();
      this.kvHandlers.set(prefix, handlers);
    }
    handlers.add(h);
    if (first && this.socket?.readyState === 1) {
      this.subscribeKV(prefix);
    }
    return () => {
      const listeners = this.kvHandlers.get(prefix);
      if (!listeners) return;
      listeners.delete(h);
      if (listeners.size !== 0) return;
      this.kvHandlers.delete(prefix);
      if (this.socket?.readyState === 1) this.unsubscribeKV(prefix);
      this.closeIfIdle();
    };
  }
  onCollection(
    collection: string,
    h: (e: { collection: string; id: string; version: number; deleted: boolean; revision: number }) => void,
  ): () => void {
    let handlers = this.collectionHandlers.get(collection);
    const first = !handlers;
    if (!handlers) {
      handlers = new Set();
      this.collectionHandlers.set(collection, handlers);
    }
    handlers.add(h);
    if (first && this.socket?.readyState === 1) {
      this.subscribeCollection(collection);
    }
    return () => {
      const listeners = this.collectionHandlers.get(collection);
      if (!listeners) return;
      listeners.delete(h);
      if (listeners.size !== 0) return;
      this.collectionHandlers.delete(collection);
      if (this.socket?.readyState === 1) this.unsubscribeCollection(collection);
      this.closeIfIdle();
    };
  }
  onStatus(handler: (status: LiveStatus) => void): () => void {
    this.statusHandlers.add(handler);
    handler(this.status);
    return () => this.statusHandlers.delete(handler);
  }
  private setStatus(status: LiveStatus) {
    if (this.status === status) return;
    this.status = status;
    this.statusHandlers.forEach((handler) => handler(status));
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
    this.setStatus(this.status === "offline" ? "connecting" : "reconnecting");
    const s = this.socket = this.options.webSocket(
      this.options.origin.replace(/^http/, "ws") + "/_tinker/ws/v1",
      `tinker.sdk.${SDK_VERSION}.api.${APP_API_VERSION}`,
    );
    s.onopen = () => {
      this.setStatus("connected");
      if (this.name && this.subscribed) {
        s.send(JSON.stringify({ v: 1, type: "subscribe", channel: this.name }));
      }
      for (const prefix of this.kvHandlers.keys()) {
        this.subscribeKV(prefix);
      }
      for (const collection of this.collectionHandlers.keys()) {
        this.subscribeCollection(collection);
      }
      this.pending?.resolve();
      this.pending = undefined;
    };
    s.onclose = () => {
      this.socket = undefined;
      if (this.pending) {
        this.pending.reject(new TinkerTemporarilyUnavailableError("Live connection closed.", "temporarily_unavailable"));
        this.pending = undefined;
      }
      if (!this.wanted) return;
      this.setStatus("reconnecting");
      this.retry = (this.options.schedule ?? setTimeout)(() => {
        this.retry = undefined;
        this.open();
      }, 250 + Math.floor((this.options.random ?? Math.random)() * 250));
    };
    s.onerror = () => {
      if (this.pending) {
        this.pending.reject(new TinkerTemporarilyUnavailableError("Live connection failed.", "temporarily_unavailable"));
        this.pending = undefined;
      }
    };
    s.onmessage = (e) => this.receive(e.data);
  }
  private subscribeKV(prefix: string) {
    this.socket?.send(JSON.stringify({ v: 1, type: "subscribe_kv", prefix }));
  }
  private unsubscribeKV(prefix: string) {
    this.socket?.send(JSON.stringify({ v: 1, type: "unsubscribe_kv", prefix }));
  }
  private subscribeCollection(collection: string) {
    this.socket?.send(JSON.stringify({ v: 1, type: "subscribe_collection", collection }));
  }
  private unsubscribeCollection(collection: string) {
    this.socket?.send(JSON.stringify({ v: 1, type: "unsubscribe_collection", collection }));
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
        for (const [prefix, handlers] of this.kvHandlers) {
          if (!d.key.startsWith(prefix)) continue;
          handlers.forEach((h) => h({
            key: d.key,
            version: Number(d.version) || 0,
            deleted: !!d.deleted,
          }));
        }
      }
      if (
        d.type === "collection.changed" &&
        typeof d.collection === "string" &&
        typeof d.id === "string"
      ) {
        this.collectionHandlers.get(d.collection)?.forEach((h) => h({
            collection: d.collection,
            id: d.id,
            version: Number(d.version) || 0,
            deleted: !!d.deleted,
            revision: Number(d.revision) || 0,
          }));
      }
    } catch {}
  }
  publish(event: string, payload: JSONValue): void {
    if (this.socket?.readyState !== 1) {
      throw new TinkerTemporarilyUnavailableError(
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
    this.setStatus("closed");
    if (this.retry !== undefined) {
      (this.options.cancel ?? ((id: unknown) => clearTimeout(id as number)))(
        this.retry,
      );
    }
    this.retry = undefined;
    if (this.pending) {
      this.pending.reject(new TinkerTemporarilyUnavailableError("Live connection closed.", "temporarily_unavailable"));
      this.pending = undefined;
    }
    this.socket?.close();
    this.socket = undefined;
  }
  private closeIfIdle(): void {
    if (!this.closeWhenIdle || this.kvHandlers.size !== 0 || this.collectionHandlers.size !== 0) return;
    this.close();
    this.onIdle?.();
  }
  get connectionStatus(): LiveStatus {
    return this.status;
  }
}
function collectionClient<T extends JSONObject>(
  name: string,
  fetcher: FetchLike,
  managedLive: () => LiveChannel,
): TinkerCollection<T> {
  const base = `/_tinker/api/v1/db/${encodeURIComponent(name)}`;
  const list = async (
    options: CollectionListOptions = {},
    snapshot = false,
  ): Promise<CollectionList<T>> => {
    const q = new URLSearchParams();
    if (options.cursor) q.set("cursor", options.cursor);
    if (options.limit) q.set("limit", String(options.limit));
    if (snapshot) q.set("snapshot", "1");
    const page = await json<{
      documents?: unknown;
      revision?: unknown;
      next_cursor?: unknown;
    }>(fetcher, `${base}?${q}`, { signal: options.signal });
    if (
      typeof page.revision !== "number" ||
      !Number.isSafeInteger(page.revision) ||
      page.revision < 0
    ) {
      throw new TinkerTemporarilyUnavailableError(
        "Tinkercloud returned an invalid collection snapshot.",
        "temporarily_unavailable",
      );
    }
    return {
      documents: Array.isArray(page.documents)
        ? page.documents.map((document) => documentMetadata<T>(document))
        : [],
      revision: page.revision,
      nextCursor: typeof page.next_cursor === "string"
        ? page.next_cursor
        : undefined,
    };
  };
  return {
    create: async (data, options: RequestOptions = {}) =>
      documentMetadata<T>(await json(fetcher, base, {
        method: "POST",
        signal: options.signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ data }),
      })),
    get: async (id, options: RequestOptions = {}) => {
      const document = await nullableJSON<unknown>(
        fetcher,
        `${base}/${encodeURIComponent(id)}`,
        { signal: options.signal },
      );
      return document === null ? null : documentMetadata<T>(document);
    },
    update: async (id, data, options: CollectionWriteOptions = {}) =>
      documentMetadata<T>(await json(fetcher, `${base}/${encodeURIComponent(id)}`, {
        method: "PUT",
        signal: options.signal,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          data,
          expected_version: options.expectedVersion,
        }),
      })),
    delete: (id, options: CollectionWriteOptions = {}) =>
      json<{ deleted: boolean }>(
        fetcher,
        `${base}/${encodeURIComponent(id)}`,
        {
          method: "DELETE",
          signal: options.signal,
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ expected_version: options.expectedVersion }),
        },
      ),
    list: (options: CollectionListOptions = {}) => list(options),
    subscribe: (handlers) => {
      const channel = managedLive();
      const lifetime = new AbortController();
      let stopped = false;
      let refreshing = false;
      let dirty = true;
      let initialized = false;
      let current = new Map<string, TinkerDocument<T>>();

      const refresh = async () => {
        if (stopped) return;
        dirty = true;
        if (refreshing) return;
        refreshing = true;
        try {
          while (dirty && !stopped) {
            dirty = false;
            const snapshot = await list({ signal: lifetime.signal }, true);
            if (stopped) return;
            const next = new Map(
              snapshot.documents.map((document) => [document.id, document]),
            );
            if (initialized) {
              for (const document of snapshot.documents) {
                const previous = current.get(document.id);
                if (!previous) {
                  handlers.onCreate?.(document);
                } else if (previous.version !== document.version) {
                  handlers.onUpdate?.(document);
                }
              }
              for (const id of current.keys()) {
                if (!next.has(id)) handlers.onDelete?.(id);
              }
            }
            current = next;
            initialized = true;
            handlers.onSnapshot(snapshot);
          }
        } catch (error) {
          if (!stopped && !(error instanceof DOMException && error.name === "AbortError")) {
            handlers.onStatus?.("error");
            handlers.onError?.(error);
          }
        } finally {
          refreshing = false;
          if (dirty && !stopped) void refresh();
        }
      };

      const offChange = channel.onCollection(name, () => {
        dirty = true;
        void refresh();
      });
      const offStatus = channel.onStatus((status) => {
        if (stopped) return;
        if (status === "connected") {
          handlers.onStatus?.("connected");
          void refresh();
        } else if (status === "reconnecting") {
          handlers.onStatus?.("reconnecting");
        } else if (status === "closed") {
          handlers.onStatus?.("closed");
        } else {
          handlers.onStatus?.("connecting");
        }
      });
      void channel.connect().catch((error) => {
        if (!stopped) {
          handlers.onStatus?.("error");
          handlers.onError?.(error);
        }
      });
      return () => {
        if (stopped) return;
        stopped = true;
        lifetime.abort();
        offStatus();
        offChange();
        handlers.onStatus?.("closed");
      };
    },
  };
}
export function createTinker(
  options: TinkerClientOptions = {},
): TinkerClient {
  const fetcher = options.fetch ?? fetch;
  const liveOptions: LiveOptions = {
    webSocket: options.webSocket ?? ((url, protocols) => new WebSocket(url, protocols)),
    origin: options.origin ?? globalThis.location?.origin ?? "",
    schedule: options.schedule,
    cancel: options.cancel,
    random: options.random,
  };
  // Capability listeners share one app-scoped socket. Explicit custom
  // channels remain independent because callers own their lifecycle directly.
  let managedLive: LiveChannel | undefined;
  const sharedLive = (): LiveChannel => {
    if (!managedLive) {
      const channel = new LiveChannel(undefined, liveOptions, true, () => {
        if (managedLive === channel) managedLive = undefined;
      });
      managedLive = channel;
    }
    return managedLive;
  };
  return {
    user: {
      current: (o: RequestOptions = {}) =>
        json<CurrentUser>(fetcher, "/_tinker/api/v1/me", { signal: o.signal }),
    },
    app: {
      info: (o: RequestOptions = {}) =>
        json<AppInfo>(fetcher, "/_tinker/api/v1/app", { signal: o.signal }),
    },
    capabilities: {
      list: (o: RequestOptions = {}) =>
        json<{ capabilities: Capability[] }>(
          fetcher,
          "/_tinker/api/v1/capabilities",
          { signal: o.signal },
        ),
    },
    llm: {
      chat: {
        // Keep the public TypeScript API idiomatic while keeping the gateway
        // wire contract explicit and stable. In particular, do not leak a
        // browser-only camelCase field into the Go JSON decoder.
        complete: async (request: ChatRequest, o: RequestOptions = {}) =>
          chatResponse(await json<unknown>(fetcher, "/_tinker/api/v1/llm/chat", {
            method: "POST", signal: o.signal,
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              messages: request.messages,
              ...(request.maxOutputTokens === undefined
                ? {}
                : { max_output_tokens: request.maxOutputTokens }),
            }),
          })),
      },
    },
    kv: {
      get: <T extends JSONValue>(key: string, o: RequestOptions = {}) =>
        nullableJSON<KVEntry<T>>(
          fetcher,
          `/_tinker/api/v1/kv/${encodeURIComponent(key)}`,
          { signal: o.signal },
        ),
      set: <T extends JSONValue>(key: string, value: T, o: SetOptions = {}) =>
        json<KVEntry<T>>(
          fetcher,
          `/_tinker/api/v1/kv/${encodeURIComponent(key)}`,
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
          `/_tinker/api/v1/kv/${encodeURIComponent(key)}`,
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
        const page = await json<KVList<T>>(fetcher, `/_tinker/api/v1/kv?${q}`, {
          signal: o.signal,
        });
        // Older servers serialized an empty Go slice as `entries: null`. Keep
        // the SDK contract iterable while callers upgrade their server.
        return { ...page, entries: Array.isArray(page.entries) ? page.entries : [] };
      },
    },
    db: {
      collection: <T extends JSONObject = JSONObject>(name: string) =>
        collectionClient<T>(name, fetcher, sharedLive),
    },
    blobs: {
      upload: async (file: File, o: RequestOptions = {}) => {
        const body = new FormData();
        // The display name is multipart metadata, never a server storage key.
        body.append("file", file, file.name);
        const response = await responseFor(fetcher, "/_tinker/api/v1/blobs", {
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
          `/_tinker/api/v1/blobs/${encodeURIComponent(id)}`,
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
          `/_tinker/api/v1/blobs?${q}`,
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
          `/_tinker/api/v1/blobs/${encodeURIComponent(id)}`,
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
        // `_tinker*` is intentionally reserved and would be rejected by the hub.
        const c = sharedLive();
        const off = c.onKv(prefix, handler);
        // A listener reconnects in the background. Consume the first failed
        // connection attempt so an unavailable live service does not create an
        // unhandled rejection in an otherwise usable KV-backed app.
        void c.connect().catch(() => {});
        return () => {
          off();
        };
      },
    },
  };
}
export const tinker: TinkerClient = createTinker();
