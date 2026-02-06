"use strict";
const node_crypto_1 = require("node:crypto");
const DEFAULT_VERSION = 1;
const DEFAULT_MAX_RETRIES = 3;
const DEFAULT_RETRY_DELAY = 250;
const DEFAULT_MAX_PAYLOAD = 1024 * 1024;
const MAX_FAILURES_BEFORE_DISABLE = 5;
const EVENT_MAPPINGS = {
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
    constructor(settings, logger) {
        this.settings = settings;
        this.logger = logger;
        this.sequence = 0;
        this.disabled = false;
        this.consecutiveFailures = 0;
    }
    async emit(type, payload) {
        if (this.disabled) {
            return;
        }
        const event = {
            version: this.settings.version,
            event_id: (0, node_crypto_1.randomUUID)(),
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
        }
        catch (err) {
            this.consecutiveFailures += 1;
            const warn = this.logger.warn ?? console.warn;
            warn("[lattice-bridge] failed to send event %s (%s). consecutive failures=%d", event.event_id, type, this.consecutiveFailures, err);
            if (this.consecutiveFailures >= MAX_FAILURES_BEFORE_DISABLE) {
                this.disabled = true;
                const error = this.logger.error ?? console.error;
                error("[lattice-bridge] disabling plugin after repeated failures. events will be dropped.");
            }
        }
    }
    trimPayload(payload) {
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
        }
        catch {
            return { truncated: true, reason: "payload could not be serialized" };
        }
    }
    async postWithRetry(event) {
        const body = JSON.stringify(event);
        let attempt = 0;
        while (attempt <= this.settings.maxRetries) {
            try {
                await this.post(body);
                return;
            }
            catch (err) {
                attempt += 1;
                if (attempt > this.settings.maxRetries) {
                    throw err;
                }
                await new Promise((resolve) => setTimeout(resolve, this.settings.retryDelayMs * Math.pow(2, attempt - 1)));
            }
        }
    }
    async post(body) {
        const headers = {
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
function resolveSettings(instance) {
    const env = process.env;
    const pluginConfig = extractPluginConfig(instance);
    const url = stringOption(env.LATTICE_BRIDGE_URL) ??
        stringOption(pluginConfig.serverUrl);
    const moduleId = stringOption(env.LATTICE_MODULE_ID) ?? stringOption(pluginConfig.moduleId);
    const workflowId = stringOption(env.LATTICE_WORKFLOW_ID) ??
        stringOption(pluginConfig.workflowId);
    const sessionId = stringOption(env.LATTICE_SESSION_ID) ??
        stringOption(pluginConfig.sessionId);
    const token = stringOption(env.LATTICE_BRIDGE_TOKEN) ?? stringOption(pluginConfig.token);
    const version = numberOption(env.LATTICE_BRIDGE_VERSION) ?? DEFAULT_VERSION;
    if (!url || !moduleId || !workflowId || !sessionId) {
        return null;
    }
    const maxRetries = numberOption(pluginConfig.maxRetries) ?? DEFAULT_MAX_RETRIES;
    const retryDelayMs = numberOption(pluginConfig.retryDelayMs) ?? DEFAULT_RETRY_DELAY;
    const maxPayloadBytes = numberOption(pluginConfig.maxPayloadBytes) ?? DEFAULT_MAX_PAYLOAD;
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
function extractPluginConfig(instance) {
    const config = instance?.config;
    if (!config) {
        return {};
    }
    const pluginNamespace = config.plugin ??
        config.plugins;
    if (pluginNamespace && typeof pluginNamespace === "object") {
        return pluginNamespace["lattice-bridge"] ?? {};
    }
    return {};
}
function stringOption(value) {
    const trimmed = typeof value === "string" ? value.trim() : "";
    return trimmed ? trimmed : undefined;
}
function numberOption(value) {
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
function latticeBridge(opencode) {
    const logger = opencode?.logger ?? console;
    const settings = resolveSettings(opencode);
    if (!settings) {
        const warn = logger.warn ?? console.warn;
        warn("[lattice-bridge] missing bridge configuration (requires LATTICE_BRIDGE_URL, LATTICE_SESSION_ID, LATTICE_MODULE_ID, LATTICE_WORKFLOW_ID). Plugin disabled.");
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
        opencode.on(eventName, (...args) => {
            const payload = safeExecute(() => transformer(...args));
            void client.emit(eventName, payload);
        });
    });
}
function safeExecute(fn) {
    try {
        return fn();
    }
    catch (err) {
        return {
            error: err instanceof Error ? err.message : "unknown transformer error",
        };
    }
}
module.exports = latticeBridge;
//# sourceMappingURL=index.js.map