import { defineConfig, devices } from '@playwright/test';

// U0 e2e: baseURL menunjuk lab serve --demo (atau vite preview).
// Playwright membuka / dan lolos axe; semua request jaringan dicegat dan
// HARUS ke 127.0.0.1 (U7 offline). Browser: chromium (loopback saja).
export default defineConfig({
  testDir: './e2e',
  use: {
    baseURL: process.env['PLAYWRIGHT_BASE_URL'] ?? 'http://127.0.0.1:8088',
  },
  projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }],
});
