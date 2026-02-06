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
declare function latticeBridge(opencode: OpenCode): void;
export = latticeBridge;
//# sourceMappingURL=index.d.ts.map