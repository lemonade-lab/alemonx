import { defineConfig, devices } from '@playwright/test'

export default defineConfig({
  testDir: './e2e',
  timeout: 15_000,
  use: {
    baseURL: 'http://127.0.0.1:4173',
    // The desktop build already ships Chrome. Keeping this explicit makes the
    // gate runnable without downloading a second browser binary in CI images.
    channel: 'chrome',
    ...devices['Desktop Chrome']
  },
  webServer: {
    command: 'yarn dev --host 127.0.0.1 --port 4173',
    url: 'http://127.0.0.1:4173',
    reuseExistingServer: !process.env.CI
  }
})
