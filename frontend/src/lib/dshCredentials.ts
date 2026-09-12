export type DSHCredentialSource = 'keyring' | 'docker-secret' | 'unavailable'

export function dshCredentialPolicy(source?: DSHCredentialSource, configured = false) {
  const readOnly = source === 'docker-secret' || source === 'unavailable'
  return {
    readOnly,
    missing: readOnly && !configured,
    placeholder: source === 'docker-secret'
      ? '由 Docker Secret 管理'
      : source === 'unavailable'
        ? '请先挂载 Docker Secret'
        : configured ? '已安全保存；留空即可沿用' : '首次连接时填写',
    description: source === 'docker-secret'
      ? '密钥由 Docker Secret 管理，不会回显；请在部署端更新 Secret。'
      : ''
  }
}
