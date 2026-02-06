import { randomUUID } from "node:crypto";

type Logger = {
  info?: (...args: unknown[]) => void;
  warn?: (...args: unknown[]) => void;
  error?: (...args: unknown[]) => void;
};

interface OpenCode {
  on(event: string, handler: (...args: unknown[]) => void): void;
  logger?: Logger;
  config?: Record<string, unknown>;
}

interface BridgeSettings {
  url: string;
  moduleId: string;
  workflowId: string;
  sessionId: string;
  token?: string;
  version: number;
  maxRetries: number;
  retryDelayMs: number;
  maxPayloadBytes: number;
}

interface BridgeEvent {
  version: number;
  event_id: string;
  sequence: number;
  type: string;
  client_time: string;
  session_id: string;
  module_id: string;
  workflow: string;
  payload: unknown;
}

const DEFAULT_VERSION = 1;
const DEFAULT_MAX_RETRIES = 3;
const DEFAULT_RETRY_DELAY = 250;
const DEFAULT_MAX_PAYLOAD = 1024 * 1024;
const MAX_FAILURES_BEFORE_DISABLE = 5;

const EVENT_MAPPINGS: Record<string, (...args: unknown[]) => unknown> = {
  "session.start": (session) => ({ session }),
  "session.end": (session, result) => ({ session, result }),
  "model.request": (request) => ({ request }),
  "model.response": (response) => ({ response }),
  "model.error": (error) => ({ error }),
  "tool.call": (tool, args) => ({ tool, args }),
  "tool.result": (tool, result) => ({ tool, result }),
  error: (error) => ({ error }),
};

class BridgeClient {
  private sequence = 0;
  private disabled = false;
  private consecutiveFailures = 0;

  constructor(
    private readonly settings: BridgeSettings,
    private readonly logger: Logger,
  ) {}

  async emit(type: string, payload: unknown): Promise<void> {
    if (this.disabled) {
      return;
    }
    const event: BridgeEvent = {
      version: this.settings.version,
      event_id: randomUUID(),
      sequence: ++this.sequence,
      type,
      client_time: new Date().toISOString(),
      session_id: this.settings.sessionId,
      module_id: this.settings.moduleId,
      workflow: this.settings.workflowId,
      payload: this.trimPayload(payload),
    };
    try {
      await this.postWithRetry(event);
      this.consecutiveFailures = 0;
    } catch (err) {
      this.consecutiveFailures += 1;
      const warn = this.logger.warn ?? console.warn;
      warn(
        "[lattice-bridge] failed to send event %s (%s). consecutive failures=%d",
        event.event_id,
        type,
        this.consecutiveFailures,
        err,
      );
      if (this.consecutiveFailures >= MAX_FAILURES_BEFORE_DISABLE) {
        this.disabled = true;
        const error = this.logger.error ?? console.error;
        error(
          "[lattice-bridge] disabling plugin after repeated failures. events will be dropped.",
        );
      }
    }
  }

  private trimPayload(payload: unknown): unknown {
    if (payload == null) {
      return null;
    }
    try {
      const text = JSON.stringify(payload);
      if (!text) {
        return payload;
      }
      const size = Buffer.byteLength(text, "utf8");
      if (size <= this.settings.maxPayloadBytes) {
        return payload;
      }
      return {
        truncated: true,
        reason: `payload exceeded ${this.settings.maxPayloadBytes} bytes (was ${size})`,
      };
    } catch {
      return { truncated: true, reason: "payload could not be serialized" };
    }
  }

  private async postWithRetry(event: BridgeEvent): Promise<void> {
    const body = JSON.stringify(event);
    let attempt = 0;
    while (attempt <= this.settings.maxRetries) {
      try {
        await this.post(body);
        return;
      } catch (err) {
        attempt += 1;
        if (attempt > this.settings.maxRetries) {
          throw err;
        }
        await new Promise((resolve) =>
          setTimeout(
            resolve,
            this.settings.retryDelayMs * Math.pow(2, attempt - 1),
          ),
        );
      }
    }
  }

  private async post(body: string): Promise<void> {
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
    };
    if (this.settings.token) {
      headers.Authorization = `Bearer ${this.settings.token}`;
    }
    const response = await fetch(this.settings.url + "/events", {
      method: "POST",
      headers,
      body,
    });
    if (!response.ok) {
      const text = await response.text();
      throw new Error(`bridge returned ${response.status}: ${text}`);
    }
  }
}

function resolveSettings(instance: OpenCode): BridgeSettings | null {
  const env = process.env;
  const pluginConfig = extractPluginConfig(instance);
  const url =
    stringOption(env.LATTICE_BRIDGE_URL) ??
    stringOption(pluginConfig.serverUrl);
  const moduleId =
    stringOption(env.LATTICE_MODULE_ID) ?? stringOption(pluginConfig.moduleId);
  const workflowId =
    stringOption(env.LATTICE_WORKFLOW_ID) ??
    stringOption(pluginConfig.workflowId);
  const sessionId =
    stringOption(env.LATTICE_SESSION_ID) ??
    stringOption(pluginConfig.sessionId);
  const token =
    stringOption(env.LATTICE_BRIDGE_TOKEN) ?? stringOption(pluginConfig.token);
  const version = numberOption(env.LATTICE_BRIDGE_VERSION) ?? DEFAULT_VERSION;
  if (!url || !moduleId || !workflowId || !sessionId) {
    return null;
  }
  const maxRetries =
    numberOption(pluginConfig.maxRetries) ?? DEFAULT_MAX_RETRIES;
  const retryDelayMs =
    numberOption(pluginConfig.retryDelayMs) ?? DEFAULT_RETRY_DELAY;
  const maxPayloadBytes =
    numberOption(pluginConfig.maxPayloadBytes) ?? DEFAULT_MAX_PAYLOAD;
  return {
    url: url.replace(/\/$/, ""),
    moduleId,
    workflowId,
    sessionId,
    token,
    version,
    maxRetries,
    retryDelayMs,
    maxPayloadBytes,
  };
}

function extractPluginConfig(instance: OpenCode): Record<string, any> {
  const config = instance?.config as Record<string, any> | undefined;
  if (!config) {
    return {};
  }
  const pluginNamespace =
    (config.plugin as Record<string, any>) ??
    (config.plugins as Record<string, any>);
  if (pluginNamespace && typeof pluginNamespace === "object") {
    return pluginNamespace["lattice-bridge"] ?? {};
  }
  return {};
}

function stringOption(value: unknown): string | undefined {
  const trimmed = typeof value === "string" ? value.trim() : "";
  return trimmed ? trimmed : undefined;
}

function numberOption(value: unknown): number | undefined {
  if (typeof value === "number" && !Number.isNaN(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim() !== "") {
    const parsed = Number(value);
    if (!Number.isNaN(parsed)) {
      return parsed;
    }
  }
  return undefined;
}

function latticeBridge(opencode: OpenCode): void {
  const logger = opencode?.logger ?? console;
  const settings = resolveSettings(opencode);
  if (!settings) {
    const warn = logger.warn ?? console.warn;
    warn(
      "[lattice-bridge] missing bridge configuration (requires LATTICE_BRIDGE_URL, LATTICE_SESSION_ID, LATTICE_MODULE_ID, LATTICE_WORKFLOW_ID). Plugin disabled.",
    );
    return;
  }
  if (typeof fetch !== "function") {
    const warn = logger.warn ?? console.warn;
    warn("[lattice-bridge] fetch API is not available. Plugin disabled.");
    return;
  }
  const client = new BridgeClient(settings, logger);
  const info = logger.info ?? console.info;
  info("[lattice-bridge] streaming events to %s", settings.url);
  Object.entries(EVENT_MAPPINGS).forEach(([eventName, transformer]) => {
    opencode.on(eventName, (...args: unknown[]) => {
      const payload = safeExecute(() => transformer(...args));
      void client.emit(eventName, payload);
    });
  });
}

function safeExecute<T>(fn: () => T): T | { error: string } {
  try {
    return fn();
  } catch (err) {
    return {
      error: err instanceof Error ? err.message : "unknown transformer error",
    };
  }
}

export = latticeBridge;
