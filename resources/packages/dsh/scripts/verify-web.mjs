import { mkdtemp, rm, writeFile, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";
const home = await mkdtemp(join(tmpdir(), "alemonx-web-smoke-"));
const entry = join(process.cwd(), "node_modules/@deepseek-ai/dsh/lib/bin.js");
const patch = join(home, "workspace.patch.yml");
await writeFile(
  patch,
  "- insert:\n    - id: alemonx-workspace-entry\n      name: " +
    JSON.stringify(join(process.cwd(), "plugins/workspace-entry/index.js")) +
    "\n",
);
const child = spawn(
  process.execPath,
  [
    entry,
    "web",
    "--patch",
    patch,
    "--host",
    "127.0.0.1",
    "--port",
    "0",
    "--no-open",
  ],
  {
    env: {
      ...process.env,
      DSH_HOME: home,
      DEEPSEEK_API_KEY: "",
      DSH_TELEMETRY_MODE: "DISABLED",
    },
    stdio: ["ignore", "pipe", "pipe"],
  },
);
const closed = new Promise((resolve) => child.once("close", resolve));
let output = "";
try {
  const address = await new Promise((resolve, reject) => {
    const timer = setTimeout(
      () => reject(new Error("startup timed out")),
      30000,
    );
    child.stdout.on("data", (data) => {
      output += data;
      const match = /dsh web: (http:\/\/127\.0\.0\.1:\d+\/\?token=\S+)/.exec(
        output,
      );
      if (match) {
        clearTimeout(timer);
        resolve(match[1]);
      }
    });
    child.stderr.on("data", (data) => {
      output += data;
    });
    child.once("error", reject);
    child.once("exit", () => {
      clearTimeout(timer);
      reject(new Error("exited before ready"));
    });
  });
  const preferences = await readFile(join(home, "settings.yaml"), "utf8");
  if (!/preference:\s*zh/.test(preferences)) throw new Error("Chinese default was not persisted");
  const authenticated = await fetch(address, { redirect: "manual" });
  const cookie = authenticated.headers.get("set-cookie")?.split(";")[0];
  if (authenticated.status !== 303 || !cookie)
    throw new Error("token exchange failed: " + authenticated.status);
  const base = new URL(address).origin;
  const index = await fetch(base + "/", { headers: { cookie } });
  const html = await index.text();
  if (
    index.status !== 200 ||
    !html.includes("<html") ||
    !html.includes("__ALX_DSH_WORKSPACE__")
  )
    throw new Error("index failed");
  const asset = /src="([^"]+\.js)"/.exec(html)?.[1];
  if (!asset) throw new Error("missing web JS entry");
  const js = await fetch(new URL(asset, base), { headers: { cookie } });
  if (js.status !== 200) throw new Error("entry asset failed");
  console.log(
    "DSH official Web startup, authentication and frontend assets verified",
  );
} finally {
  child.kill("SIGKILL");
  await closed;
  await rm(home, { recursive: true, force: true });
}
