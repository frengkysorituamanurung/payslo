import '@testing-library/jest-dom/vitest';
import { afterAll, afterEach, beforeAll } from 'vitest';
import { setupServer } from 'msw/node';
import { handlers, resetMockState } from '../mocks/handlers';
import '../i18n';

// jsdom tidak punya matchMedia (dipakai deteksi tema awal).
Object.defineProperty(window, 'matchMedia', {
  writable: true,
  value: (query: string) => {
    const noop = (): void => undefined;
    return {
      matches: false,
      media: query,
      addEventListener: noop,
      removeEventListener: noop,
    };
  },
});

export const server = setupServer(...handlers);
beforeAll(() => {
  server.listen({ onUnhandledFrame: 'error' });
});
afterEach(() => {
  server.resetHandlers();
  resetMockState();
});
afterAll(() => {
  server.close();
});
