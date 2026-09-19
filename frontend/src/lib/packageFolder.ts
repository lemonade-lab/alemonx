export type PackageFolderFile = { path: string; file: File }
const excluded = (name: string) =>
  name.startsWith('.') || name === 'node_modules'

export function filesFromFolderPicker(files: FileList): PackageFolderFile[] {
  return Array.from(files).flatMap(file => {
    const parts = file.webkitRelativePath.split('/')
    if (parts.length < 2) throw new Error('请选择插件文件夹。')
    parts.shift()
    return parts.some(excluded) ? [] : [{ path: parts.join('/'), file }]
  })
}

export async function filesFromFolderDrop(
  items: DataTransferItemList
): Promise<PackageFolderFile[]> {
  const entries = Array.from(items)
    .filter(item => item.kind === 'file')
    .map(item => item.webkitGetAsEntry())
  if (entries.length !== 1 || !entries[0]?.isDirectory)
    throw new Error('请拖入一个插件文件夹，或点击选择文件夹。')
  const files: PackageFolderFile[] = []
  const walk = async (
    entry: FileSystemEntry,
    prefix: string
  ): Promise<void> => {
    if (excluded(entry.name)) return
    const path = prefix ? `${prefix}/${entry.name}` : entry.name
    if (entry.isFile) {
      const file = await new Promise<File>((resolve, reject) =>
        (entry as FileSystemFileEntry).file(resolve, reject)
      )
      files.push({ path, file })
      if (files.length > 10000) throw new Error('文件数量不能超过 10000。')
    } else {
      const reader = (entry as FileSystemDirectoryEntry).createReader()
      while (true) {
        const batch = await new Promise<FileSystemEntry[]>((resolve, reject) =>
          reader.readEntries(resolve, reject)
        )
        if (!batch.length) break
        for (const child of batch) await walk(child, path)
      }
    }
  }
  const reader = (entries[0] as FileSystemDirectoryEntry).createReader()
  while (true) {
    const batch = await new Promise<FileSystemEntry[]>((resolve, reject) =>
      reader.readEntries(resolve, reject)
    )
    if (!batch.length) break
    for (const entry of batch) await walk(entry, '')
  }
  return files
}

export async function validatePackageFolder(files: PackageFolderFile[]) {
  if (
    files.length > 10000 ||
    files.reduce((sum, item) => sum + item.file.size, 0) > 500 * 1024 * 1024
  )
    throw new Error('文件夹不能超过 500 MB 或 10000 个文件。')
  const manifest = files.find(item => item.path === 'package.json')
  if (!manifest || manifest.file.size > 1024 * 1024)
    throw new Error('请选择根目录包含有效 package.json 的插件文件夹。')
  let value: unknown
  try {
    value = JSON.parse(await manifest.file.text())
  } catch {
    throw new Error('package.json 格式不正确。')
  }
  if (
    !value ||
    typeof value !== 'object' ||
    !('name' in value) ||
    typeof value.name !== 'string' ||
    !value.name.trim()
  )
    throw new Error('package.json 缺少插件名称 name。')
  return value.name
}
