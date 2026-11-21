import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import vm from "node:vm";
import { apply as hostApply } from "../plugins/workspace-entry/index.js";
const clientSource = await readFile(
  new URL("../plugins/workspace-entry/client.js", import.meta.url),
  "utf8",
);

function snapshot(value) {
  const listeners = new Set();
  return {
    getSnapshot: () => value,
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    set(next) {
      value = next;
      for (const listener of listeners) listener();
    },
    notify() {
      for (const listener of listeners) listener();
    },
    get count() {
      return listeners.size;
    },
  };
}
function client(
  workspaceState,
  sessionState,
  openWorkspace = async () => {},
  { embedded = false, createWorkspace = async () => {} } = {},
) {
  let factory;
  const messages = [];
  const window = Object.assign(new EventTarget(), {
    location: { origin: "http://127.0.0.1:4242" },
    __ALX_DSH_WORKSPACE__: { workspaceId: "project" },
    __ModuleLoader__: {
      load: (value) => {
        factory = value.factory;
      },
    },
  });
  window.parent = embedded
    ? { postMessage: ({ type }, origin) => messages.push({ type, origin }) }
    : window;
  vm.runInNewContext(clientSource, { window });

  const imported = [];
  const workspaces = {
    list: snapshot(workspaceState),
    async create({ path }) {
      imported.push(path);
      await createWorkspace(path);
    },
  };
  const sessions = { list: snapshot(sessionState) };
  const opened = [],
    effects = [],
    events = new Map();
  const services = {
    workspaces,
    sessions,
    uiWorkspace: { openWorkspace, openSession: (id) => opened.push(id) },
  };
  factory().apply({
    get: (key) => services[key],
    effect: (setup) => effects.push(setup()),
    on: (name, listener) => {
      events.set(name, listener);
      return () => events.delete(name);
    },
  });
  return {
    workspaces,
    sessions,
    opened,
    events,
    imported,
    messages,
    receive(roots, { source = window.parent, origin = window.location.origin } = {}) {
      window.dispatchEvent(Object.assign(new Event("message"), {
        source,
        origin,
        data: { type: "alx:dsh-robot-workspaces", roots },
      }));
    },
    dispose: () => effects.forEach((effect) => effect()),
  };
}
const workspaceState = {
  phase: "ready",
  archivedSessionIds: ["archived"],
  items: [
    { workspaceId: "other", sessionIds: ["foreign"] },
    { workspaceId: "project", sessionIds: ["old", "recent", "archived"] },
  ],
};
const sessionState = {
  phase: "ready",
  current: "foreign",
  byId: {
    old: { id: "old", updatedAt: 1 },
    recent: { id: "recent", updatedAt: 2 },
    archived: { id: "archived", updatedAt: 99 },
    foreign: { id: "foreign", updatedAt: 100 },
  },
};
const restored = client(workspaceState, sessionState);
assert.deepEqual(restored.opened, ["recent"]);
restored.sessions.list.notify();
restored.events.get("connection/reset")();
assert.deepEqual(
  restored.opened,
  ["recent"],
  "reconnect must not override subsequent user navigation",
);
restored.dispose();
assert.equal(restored.sessions.list.count, 0);
assert.equal(restored.workspaces.list.count, 0);
let opens = 0,
  release;
const empty = client(
  { ...workspaceState, items: [{ workspaceId: "project", sessionIds: [] }] },
  { phase: "pending", byId: {} },
  () => {
    opens++;
    return new Promise((resolve) => {
      release = resolve;
    });
  },
);
assert.equal(opens, 0);
empty.sessions.list.set({ phase: "ready", byId: {} });
empty.workspaces.list.notify();
assert.equal(
  opens,
  1,
  "baseline bursts must not create multiple blank sessions",
);
release();
await new Promise((resolve) => setTimeout(resolve, 0));
empty.sessions.list.notify();
assert.equal(opens, 1);
empty.dispose();
let attempts = 0;
const retry = client(
  { ...workspaceState, items: [{ workspaceId: "project", sessionIds: [] }] },
  { phase: "ready", byId: {} },
  async () => {
    if (++attempts === 1) throw new Error("disconnected");
  },
);
await new Promise((resolve) => setTimeout(resolve, 0));
retry.events.get("connection/reset")();
await new Promise((resolve) => setTimeout(resolve, 0));
assert.equal(attempts, 2);
retry.dispose();
const disposed = client(
  { phase: "pending", items: [] },
  { phase: "pending", byId: {} },
);
disposed.dispose();
disposed.workspaces.list.set(workspaceState);
disposed.sessions.list.set(sessionState);
assert.deepEqual(disposed.opened, []);
let importAttempts = 0;
const embedded = client(workspaceState, sessionState, undefined, {
  embedded: true,
  async createWorkspace(path) {
    if (path === "/retry" && ++importAttempts === 1)
      throw new Error("disconnected");
  },
});
assert.deepEqual(embedded.messages, [
  { type: "alx:dsh-workspaces-ready", origin: "http://127.0.0.1:4242" },
]);
embedded.receive(["/wrong-origin"], { origin: "http://localhost:4242" });
embedded.receive(["/wrong-source"], { source: {} });
embedded.receive(["/robot-a", null, "", "/robot-a", "/robot-b"]);
embedded.receive(["/robot-b", "/robot-c"]);
assert.deepEqual(embedded.imported, ["/robot-a", "/robot-b", "/robot-c"]);
assert.deepEqual(embedded.opened, ["recent"], "importing folders must not switch the current session");
embedded.receive(["/retry"]);
await new Promise(setImmediate);
embedded.receive(["/retry", "/robot-a"]);
assert.equal(importAttempts, 2, "a failed workspace import is retryable");
const importedBeforeDispose = embedded.imported.slice();
embedded.dispose();
embedded.receive(["/after-dispose"]);
assert.deepEqual(embedded.imported, importedBeforeDispose);

const standalone = client(workspaceState, sessionState);
standalone.receive(["/standalone"]);
assert.deepEqual(standalone.messages, []);
assert.deepEqual(standalone.imported, []);
standalone.dispose();

let registered, injectIndex;
const localeWrites = [];
const settings = {
  get: () => ({}),
  update: async (namespace, patch) => localeWrites.push({ namespace, patch }),
};
await hostApply({
  settings,
  workspaceRegistry: {
    archivedSessionIds: [],
    async create(path) {
      registered = path;
      return { id: "canonical-id", path: "/canonical/project", sessionIds: [] };
    },
  },
  sessionController: {
    async create(request) {
      assert.equal(request.workspaceId, "canonical-id");
    },
  },
  on(name, listener) {
    assert.equal(name, "webserver/index-inject");
    injectIndex = listener;
  },
});
assert.equal(registered, process.cwd());
assert.deepEqual(localeWrites, [{ namespace: "locale", patch: { preference: "zh" } }]);
const table = [];
injectIndex(table);
assert.deepEqual(table, [
  {
    kind: "global",
    name: "__ALX_DSH_WORKSPACE__",
    value: { workspaceId: "canonical-id", path: "/canonical/project" },
  },
]);

await hostApply({
  settings: {
    get: () => ({ preference: "en" }),
    update() { throw new Error("must preserve explicit language choice"); },
  },
  workspaceRegistry: {
    archivedSessionIds: ["old"],
    async create() {
      return {
        id: "existing",
        path: "/canonical/project",
        sessionIds: ["old", "active"],
      };
    },
  },
  sessionController: {
    create() {
      throw new Error("existing workspace must not get another blank session");
    },
  },
  on() {},
});

console.log(
  "DSH workspace entry: project selection, recent session, archive exclusion, deduplication, reconnect, embedded directory import and disposal verified",
);
