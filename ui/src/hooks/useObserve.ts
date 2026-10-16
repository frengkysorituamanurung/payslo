import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  api,
  type AlertmanagerAlert,
  type Annotation,
  type DiagnosticsReport,
  type GrafanaEmbedInfo,
  type NtfyHistory,
  type ObserveStatus,
  type PrometheusAlert,
  type PrometheusRuleGroup,
} from '../api/client';
import { ApiError } from './usePayments';

export type {
  AlertmanagerAlert,
  Annotation,
  DiagnosticsReport,
  GrafanaEmbedInfo,
  NtfyHistory,
  ObserveStatus,
  PrometheusAlert,
  PrometheusRuleGroup,
};

function throwProblem(error: unknown, status: number): never {
  const p = error as { detail?: string; error?: string } | undefined;
  throw new ApiError(status, p?.detail ?? p?.error ?? 'request failed');
}

export function useObserveStatus(): UseQueryResult<ObserveStatus, ApiError> {
  return useQuery({
    queryKey: ['observe-status'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/status');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 15_000,
  });
}

export function useGrafanaEmbed(): UseQueryResult<GrafanaEmbedInfo, ApiError> {
  return useQuery({
    queryKey: ['grafana-embed'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/grafana');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    staleTime: 60_000,
  });
}

export function useAnnotations(): UseQueryResult<Annotation[], ApiError> {
  return useQuery({
    queryKey: ['annotations'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/annotations');
      if (data === undefined) throwProblem(error, response.status);
      return data.annotations;
    },
    refetchInterval: 30_000,
  });
}

export function usePrometheusAlerts(): UseQueryResult<PrometheusAlert[], ApiError> {
  return useQuery({
    queryKey: ['prom-alerts'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/prometheus/alerts');
      if (data === undefined) throwProblem(error, response.status);
      return data.alerts;
    },
    refetchInterval: 15_000,
  });
}

export function usePrometheusRules(): UseQueryResult<PrometheusRuleGroup[], ApiError> {
  return useQuery({
    queryKey: ['prom-rules'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/prometheus/rules');
      if (data === undefined) throwProblem(error, response.status);
      return data.groups;
    },
    refetchInterval: 30_000,
  });
}

export function useAlertmanager(): UseQueryResult<AlertmanagerAlert[], ApiError> {
  return useQuery({
    queryKey: ['alertmanager'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/alertmanager');
      if (data === undefined) throwProblem(error, response.status);
      return data.alerts;
    },
    refetchInterval: 15_000,
  });
}

export function useNtfyHistory(): UseQueryResult<NtfyHistory, ApiError> {
  return useQuery({
    queryKey: ['ntfy-history'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/observe/ntfy/history');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 15_000,
  });
}

export function useDiagnostics(): UseQueryResult<DiagnosticsReport, ApiError> {
  return useQuery({
    queryKey: ['diagnostics'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/diagnostics');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useCopyDiagnostics(): UseMutationResult<string, ApiError, DiagnosticsReport> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (report: DiagnosticsReport) => {
      const text = JSON.stringify(report, null, 2);
      await navigator.clipboard.writeText(text);
      return text;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['diagnostics'] });
    },
  });
}
