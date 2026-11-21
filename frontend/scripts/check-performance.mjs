import { mkdtemp, readFile, rm, stat } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { build } from 'vite'

const entrypoints = ['index.html', 'src/renderApp.tsx']
const scenarios = [
  {
    name: '登录首屏',
    roots: entrypoints,
    budget: { raw: 730, gzip: 195, brotli: 180 }
  },
  {
    name: '工作台首屏',
    roots: [...entrypoints, 'src/App.tsx'],
    budget: { raw: 1225, gzip: 350, brotli: 320 }
  }
]

function collectAssets(manifest, roots) {
  const assets = new Set()
  const visited = new Set()
  function visit(id) {
    if (visited.has(id)) return
    visited.add(id)
    const entry = manifest[id]
    if (!entry) throw new Error(`找不到首屏构建入口 ${id}`)
    if (/\.(js|css)$/.test(entry.file)) assets.add(entry.file)
    for (const file of entry.css ?? []) assets.add(file)
    for (const imported of entry.imports ?? []) visit(imported)
  }
  roots.forEach(visit)
  return assets
}

async function compressedSize(path, fallback) {
  try {
    return (await stat(path)).size
  } catch (error) {
    if (error.code !== 'ENOENT') throw error
    return fallback
  }
}

async function measure(outDir, assets) {
  const sizes = await Promise.all(
    [...assets].map(async file => {
      const path = join(outDir, file)
      const raw = (await stat(path)).size
      const gzip = await compressedSize(path + '.gz', raw)
      const brotli = await compressedSize(path + '.br', gzip)
      return { raw, gzip, brotli }
    })
  )
  return Object.fromEntries(
    ['raw', 'gzip', 'brotli'].map(key => [
      key,
      sizes.reduce((sum, size) => sum + size[key], 0)
    ])
  )
}

const outDir = await mkdtemp(join(tmpdir(), 'alemonx-performance-'))
try {
  await build({ logLevel: 'warn', build: { outDir, manifest: true } })
  const manifest = JSON.parse(
    await readFile(join(outDir, '.vite/manifest.json'), 'utf8')
  )
  const report = []
  const failures = []
  for (const { name, roots, budget } of scenarios) {
    const assets = collectAssets(manifest, roots)
    const sizes = await measure(outDir, assets)
    for (const [encoding, size] of Object.entries(sizes)) {
      if (size > budget[encoding] * 1024)
        failures.push(`${name} ${encoding} 超过 ${budget[encoding]} KiB`)
    }
    const sizeText = key =>
      `${(sizes[key] / 1024).toFixed(1)} / ${budget[key]} KiB`
    report.push({
      场景: name,
      资源数: assets.size,
      原始: sizeText('raw'),
      Gzip: sizeText('gzip'),
      Brotli: sizeText('brotli')
    })
  }
  console.table(report)
  if (failures.length) {
    console.error(failures.join('\n'))
    process.exitCode = 1
  }
} finally {
  await rm(outDir, { recursive: true, force: true })
}
