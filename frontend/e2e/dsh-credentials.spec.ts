import { expect, test } from '@playwright/test'
import { dshCredentialPolicy } from '../src/lib/dshCredentials'

test('Docker Secret stays read-only before and after configuration', () => {
  expect(dshCredentialPolicy('docker-secret', false)).toMatchObject({ readOnly: true, missing: true })
  expect(dshCredentialPolicy('docker-secret', true)).toMatchObject({ readOnly: true, missing: false, placeholder: '由 Docker Secret 管理' })
  expect(dshCredentialPolicy('docker-secret', true).description).not.toContain('钥匙串')
})

test('Docker without a Secret requests deployment configuration, not keyring input', () => {
  expect(dshCredentialPolicy('unavailable')).toMatchObject({ readOnly: true, missing: true, placeholder: '请先挂载 Docker Secret' })
})

test('native and older servers retain editable keyring credentials', () => {
  expect(dshCredentialPolicy('keyring', true)).toMatchObject({ readOnly: false, missing: false })
  expect(dshCredentialPolicy()).toMatchObject({ readOnly: false, missing: false })
})
