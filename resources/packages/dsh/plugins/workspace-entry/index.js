// The working directory is supplied by the managed Go process, never by an
// embedded page. DSH canonicalizes it and reuses its durable workspace record.
export const inject = ["workspaceRegistry", "webServer", "sessionController", "settings"];

export async function apply(ctx) {
  const locale = ctx.settings.get("locale");
  if (locale && locale.preference === undefined) {
    // Seed the official preference once; later user selections remain authoritative.
    await ctx.settings.update("locale", { preference: "zh" });
  }
  const workspace = await ctx.workspaceRegistry.create(process.cwd());
  const archived = new Set(ctx.workspaceRegistry.archivedSessionIds);
  if (!workspace.sessionIds.some((id) => !archived.has(id))) {
    // Prepare one blank session before any page can load. Concurrent first
    // opens then see the same official session instead of racing to create it.
    await ctx.sessionController.create({ workspaceId: workspace.id });
  }
  ctx.on("webserver/index-inject", (table) => {
    table.push({
      kind: "global",
      name: "__ALX_DSH_WORKSPACE__",
      value: { workspaceId: String(workspace.id), path: workspace.path },
    });
  });
}
