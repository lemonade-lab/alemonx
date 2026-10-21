// Download isolated, integrity-checked CLIs; never install global packages or
// run a package manager against a real robot project.
import { createHash } from 'node:crypto';
import { execFileSync, spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, writeFile, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const project = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const temporary = await mkdtemp(join(tmpdir(), 'alx-package-audit-'));
const matrix = [
  ['yarn', '1.22.22', 'yarn', 'bin/yarn.js'],
  ['@yarnpkg/cli-dist', '4.9.2', 'yarn', 'bin/yarn.js'],
  ['pnpm', '9.15.0', 'pnpm', 'bin/pnpm.cjs'],
  ['pnpm', '10.17.1', 'pnpm', 'bin/pnpm.cjs'],
  ['npm', '10.9.8', 'npm', 'bin/npm-cli.js'],
  ['npm', '11.6.2', 'npm', 'bin/npm-cli.js'],
];

async function download(url) {
  const response = await fetch(url, { signal: AbortSignal.timeout(45_000) });
  if (!response.ok) throw new Error(`Download failed: ${response.status} ${url}`);
  return response;
}

let status = 1;
try {
  const tools = await Promise.all(matrix.map(async ([name, version, manager, entry]) => {
    const metadata = await (await download(`https://registry.npmjs.org/${name}/${version}`)).json();
    const archive = Buffer.from(await (await download(metadata.dist.tarball)).arrayBuffer());
    const [algorithm, expected] = metadata.dist.integrity.split('-');
    if (createHash(algorithm).update(archive).digest('base64') !== expected) {
      throw new Error(`Integrity mismatch for ${name}@${version}`);
    }
    const directory = join(temporary, `${manager}-${version}`);
    await mkdir(directory);
    const filename = join(directory, 'package.tgz');
    await writeFile(filename, archive);
    execFileSync('tar', ['-xzf', filename, '-C', directory]);
    return { manager, version, entry: join(directory, 'package', entry) };
  }));
  const filename = join(temporary, 'tools.json');
  await writeFile(filename, JSON.stringify(tools));
  const result = spawnSync('go', ['test', './internal/robot', '-run', '^TestPackageCLISmoke$', '-v', '-count=1'], {
    cwd: project,
    env: { ...process.env, ALEMONX_PACKAGE_CLI_SMOKE: filename },
    stdio: 'inherit',
  });
  if (result.error) throw result.error;
  status = result.status ?? 1;
} finally {
  await rm(temporary, { recursive: true, force: true });
}
process.exitCode = status;
