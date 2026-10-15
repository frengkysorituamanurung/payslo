import { useQuery } from '@tanstack/react-query';
import { api, type Session, type StackHealth } from '../api/client';
import type { components } from '../api/schema';

type WindowParam = NonNullable<components['parameters']['WindowParam']>;
type MetricName = components['schemas']['MetricName'];
type ProviderName = components['schemas']['ProviderName'];
type DemoScenario = components['schemas']['DemoScenario'];
export type MetricValue = components['schemas']['MetricValue'];
export type MetricsSummary = components['schemas']['MetricsSummary'];
export type MetricsSeries = components['schemas']['MetricsSeries'];

function isSeriesPoint(p: unknown): p is [string, number | null] {
  if (!Array.isArray(p) || p.length !== 2) return false;
  const [ts, val]: unknown[] = p as unknown[];
  return typeof ts === 'string' && (typeof val === 'number' || val === null);
}

export function useSession() {
  return useQuery({
    queryKey: ['session'],
    queryFn: async (): Promise<Session> => {
      const { data } = await api.GET('/api/v1/session');
      if (data === undefined) throw new Error('session failed');
      return data;
    },
    refetchInterval: 10_000,
  });
}

export function useStackHealth(enabled = true) {
  return useQuery({
    queryKey: ['stack'],
    queryFn: async (): Promise<StackHealth> => {
      const { data } = await api.GET('/api/v1/health/stack');
      if (data === undefined) throw new Error('stack failed');
      return data;
    },
    refetchInterval: 5_000,
    enabled,
  });
}

export function useMetricsSummary(window: WindowParam = '5m', demo?: DemoScenario) {
  return useQuery({
    queryKey: ['metrics-summary', window, demo ?? 'live'],
    queryFn: async (): Promise<MetricsSummary> => {
      const { data } = await api.GET('/api/v1/metrics/summary', {
        params: { query: { window, ...(demo === undefined ? {} : { demo }) } },
      });
      if (data === undefined) throw new Error('summary failed');
      return data;
    },
    refetchInterval: 5_000,
  });
}

export function useMetricsSeries(
  name: MetricName,
  window: WindowParam = '15m',
  provider?: ProviderName,
) {
  return useQuery({
    queryKey: ['metrics-series', name, window, provider ?? ''],
    queryFn: async (): Promise<MetricsSeries> => {
      const { data } = await api.GET('/api/v1/metrics/series', {
        params: { query: { name, window, ...(provider === undefined ? {} : { provider }) } },
      });
      if (data === undefined) throw new Error('series failed');
      // Normalisasi + validasi titik (kontrak: [RFC3339, number|null]).
      // `points` wajib ada menurut kontrak; Array.isArray menjaga runtime.
      const points: Array<[string, number | null]> = [];
      const rawPoints: unknown = data.points;
      if (Array.isArray(rawPoints)) {
        for (const p of rawPoints) {
          if (!isSeriesPoint(p)) continue;
          points.push(p);
        }
      }
      return { ...data, points };
    },
    refetchInterval: 5_000,
  });
}
