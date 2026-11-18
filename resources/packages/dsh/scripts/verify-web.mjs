import { mkdtemp, rm, writeFile, readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawn } from "node:child_process";

const startupTimeout = Number(process.env.DSH_WEB_STARTUP_TIMEOUT_MS || 30_000);
if (
  !Number.isSafeInteger(startupTimeout) ||
  startupTimeout < 1 ||
  startupTimeout > 2_147_483_647
) {
  throw new Error("DSH_WEB_STARTUP_TIMEOUT_MS must be a valid timeout in milliseconds");
}

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
const ready = Promise.withResolvers();
const closed = new Promise((resolve) => {
  child.once("close", (code, signal) => {
    const reason = signal || `exit code ${code}`;
    ready.reject(new Error(`exited before ready (${reason})`));
    resolve();
  });
});
child.once("error", ready.reject);
const output = { stdout: "", stderr: "" };
const readyAddress = /dsh web: (http:\/\/127\.0\.0\.1:\d+\/\?token=\S+)(?=\s)/;
for (const stream of ["stdout", "stderr"]) {
  child[stream].setEncoding("utf8");
  child[stream].on("data", (data) => {
    output[stream] = (output[stream] + data).slice(-8192);
    const match = readyAddress.exec(output[stream]);
    if (match) ready.resolve(match[1]);
  });
}
const timer = setTimeout(
  () => ready.reject(new Error(`startup timed out after ${startupTimeout}ms`)),
  startupTimeout,
);
try {
  const address = await ready.promise;
  clearTimeout(timer);
  const preferences = await readFile(join(home, "settings.yaml"), "utf8");
  if (!/preference:\s*zh/.test(preferences)) throw new Error("Chinese default was not persisted");
  const authenticated = await fetch(address, {
    redirect: "manual",
    signal: AbortSignal.timeout(30_000),
  });
  const cookie = authenticated.headers.get("set-cookie")?.split(";")[0];
  if (authenticated.status !== 303 || !cookie)
    throw new Error("token exchange failed: " + authenticated.status);
  const base = new URL(address).origin;
  const index = await fetch(base + "/", {
    headers: { cookie },
    signal: AbortSignal.timeout(30_000),
  });
  const html = await index.text();
  if (
    index.status !== 200 ||
    !html.includes("<html") ||
    !html.includes("__ALX_DSH_WORKSPACE__")
  )
    throw new Error("index failed");
  const asset = /src="([^"]+\.js)"/.exec(html)?.[1];
  if (!asset) throw new Error("missing web JS entry");
  const js = await fetch(new URL(asset, base), {
    headers: { cookie },
    signal: AbortSignal.timeout(30_000),
  });
  if (js.status !== 200) throw new Error("entry asset failed");
  console.log(
    "DSH official Web startup, authentication and frontend assets verified",
  );
} catch (error) {
  const details = Object.entries(output)
    .filter(([, text]) => text.trim())
    .map(([stream, text]) => {
      const redacted = text.trim().replace(/([?&]token=)[^\s&"'<>]+/gi, "$1[redacted]");
      return `DSH ${stream}:\n${redacted}`;
    });
  const platform = `${process.platform}/${process.arch}`;
  throw new Error([`DSH Web smoke check failed on ${platform}: ${error.message}`, ...details].join("\n"));
} finally {
  clearTimeout(timer);
  child.kill("SIGKILL");
  await closed;
  await rm(home, { recursive: true, force: true });
}
