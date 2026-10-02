window.__ModuleLoader__.load({
  id: "@alemonx/dsh-workspace-entry",
  factory: () => ({
    inject: ["sessions", "workspaces", "uiWorkspace"],
    apply(ctx) {
      const target = window.__ALX_DSH_WORKSPACE__;
      if (!target || typeof target.workspaceId !== "string") {
        throw new Error("无法读取当前项目的 DSH 工作区，请重新打开 Web 版。");
      }
      const sessions = ctx.get("sessions");
      const workspaces = ctx.get("workspaces");
      const navigation = ctx.get("uiWorkspace");
      let disposed = false;
      let opening = false;
      let selected = false;
      const reconcile = () => {
        if (disposed || opening || selected) return;
        const workspaceState = workspaces.list.getSnapshot();
        const sessionState = sessions.list.getSnapshot();
        if (workspaceState.phase !== "ready" || sessionState.phase !== "ready")
          return;
        const workspace = workspaceState.items.find(
          (item) => item.workspaceId === target.workspaceId,
        );
        if (!workspace) return;
        const archived = new Set(workspaceState.archivedSessionIds);
        const latest = workspace.sessionIds
          .filter((id) => !archived.has(id) && sessionState.byId[id])
          .map((id) => sessionState.byId[id])
          .sort((a, b) => b.updatedAt - a.updatedAt)[0];
        if (latest) {
          selected = true;
          navigation.openSession(latest.id);
          return;
        }
        opening = true;
        navigation
          .openWorkspace(workspace.workspaceId)
          .then(() => {
            if (!disposed) selected = true;
          })
          .catch(() => {
            // A reconnect publishes new snapshots and retries this initial open.
          })
          .finally(() => {
            opening = false;
          });
      };
      const stopWorkspaces = workspaces.list.subscribe(reconcile);
      const stopSessions = sessions.list.subscribe(reconcile);
      const stopReconnect = ctx.on("connection/reset", reconcile);
      ctx.effect(
        () => () => {
          disposed = true;
          stopWorkspaces();
          stopSessions();
          stopReconnect();
        },
        "alemonx.workspace-entry",
      );
      reconcile();
    },
  }),
});
