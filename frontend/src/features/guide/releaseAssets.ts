import type { ReleaseAsset } from './types'

export function recommendReleaseAssets(
  assets: ReleaseAsset[],
  userAgent: string
) {
  const platform = userAgent.includes('win')
    ? 'windows'
    : userAgent.includes('mac')
      ? 'macos'
      : userAgent.includes('freebsd')
        ? 'freebsd'
      : 'linux'
  const architecture = /arm64|aarch64/.test(userAgent)
    ? 'arm64'
    : /riscv64/.test(userAgent)
      ? 'riscv64'
      : /ppc64le/.test(userAgent)
        ? 'ppc64le'
        : /s390x/.test(userAgent)
          ? 's390x'
          : /armv?[5-8]|armhf/.test(userAgent)
            ? 'arm'
            : /i[3-6]86|x86(?![_-]?64)/.test(userAgent)
              ? '386'
              : /x86_64|\bx64\b|win64|wow64|intel mac/.test(userAgent)
                ? 'amd64'
                : ''
  const tokens = (asset: ReleaseAsset) =>
    new Set(
      asset.name
        .toLowerCase()
        .split(/[^a-z0-9_]+/)
        .filter(Boolean)
    )
  const isMetadata = (asset: ReleaseAsset) =>
    /\.sha\d*|\.sig|checksums?|latest\.yml/.test(asset.name.toLowerCase())
  const matchesSystem = (asset: ReleaseAsset) => {
    const values = tokens(asset)
    return (
      (platform === 'windows' &&
        (values.has('windows') || values.has('win32'))) ||
      (platform === 'macos' &&
        (values.has('macos') ||
          values.has('mac') ||
          values.has('darwin') ||
          values.has('osx'))) ||
      (platform === 'linux' &&
        (values.has('linux') ||
          values.has('appimage') ||
          values.has('deb') ||
          values.has('rpm'))) ||
      (platform === 'freebsd' && values.has('freebsd'))
    )
  }
  const matchesArchitecture = (asset: ReleaseAsset) => {
    const values = tokens(asset)
    const aliases: Record<string, string[]> = {
      amd64: ['x64', 'amd64', 'x86_64'],
      arm64: ['arm64', 'aarch64'],
      386: ['386', 'i386', 'x86'],
      arm: ['arm', 'armv7', 'armv7l', 'armhf'],
      ppc64le: ['ppc64le'],
      s390x: ['s390x'],
      riscv64: ['riscv64']
    }
    return aliases[architecture]?.some(value => values.has(value)) ?? false
  }
  const downloadable = assets.filter(asset => !isMetadata(asset))
  const hasRecommendation = downloadable.some(
    asset => matchesSystem(asset) && matchesArchitecture(asset)
  )
  const sorted = downloadable
    .slice()
    .sort(
      (left, right) =>
        Number(matchesSystem(right)) - Number(matchesSystem(left)) ||
        Number(matchesArchitecture(right)) - Number(matchesArchitecture(left))
    )

  return {
    assets: sorted,
    isRecommended: (asset: ReleaseAsset) =>
      hasRecommendation && matchesSystem(asset) && matchesArchitecture(asset)
  }
}
