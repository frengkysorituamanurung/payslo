import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  api,
  type MentorAssessment,
  type MentorSessionInfo,
  type MentorSessionSummary,
  type MentorTimelineEntry,
  type SnapshotInfo,
  type TrafficState,
} from '../api/client';
import { ApiError } from './usePayments';

export type {
  MentorAssessment,
  MentorSessionInfo,
  MentorSessionSummary,
  MentorTimelineEntry,
  SnapshotInfo,
};

function throwProblem(error: unknown, status: number): never {
  const p = error as { detail?: string; error?: string } | undefined;
  throw new ApiError(status, p?.detail ?? p?.error ?? 'request failed');
}

// ---------- mentor gate ----------

export function useMentorUnlock(): UseMutationResult<
  { ok: boolean; token: string },
  ApiError,
  string
> {
  return useMutation({
    mutationFn: async (pin: string) => {
      const { data, error, response } = await api.POST('/api/v1/mentor/unlock', {
        body: { pin },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useMentorLock(): UseMutationResult<unknown, ApiError, void> {
  return useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.POST('/api/v1/mentor/lock');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

// ---------- timeline ----------

export function useMentorTimeline(limit = 100): UseQueryResult<MentorTimelineEntry[], ApiError> {
  return useQuery({
    queryKey: ['mentor-timeline', limit],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/mentor/timeline', {
        params: { query: { limit } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data.entries;
    },
    refetchInterval: 10_000,
  });
}

// ---------- inject traffic ----------

export function useMentorInjectTraffic(): UseMutationResult<
  TrafficState,
  ApiError,
  { ratePerSec: number; secs?: number }
> {
  return useMutation({
    mutationFn: async ({ ratePerSec, secs }) => {
      const { data, error, response } = await api.POST('/api/v1/mentor/inject/traffic', {
        body: { ratePerSec, ...(secs === undefined ? {} : { secs }) },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

// ---------- snapshot ----------

export function useSnapshots(): UseQueryResult<SnapshotInfo[], ApiError> {
  return useQuery({
    queryKey: ['mentor-snapshots'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/mentor/snapshots');
      if (data === undefined) throwProblem(error, response.status);
      return data.snapshots;
    },
  });
}

export function useSnapshotSave(): UseMutationResult<
  { name: string; savedAt: string; dataTables: number },
  ApiError,
  string
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => {
      const { data, error, response } = await api.POST('/api/v1/mentor/snapshot/save', {
        body: { name },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['mentor-snapshots'] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useSnapshotLoad(): UseMutationResult<
  { name: string; restored: string[] },
  ApiError,
  string
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => {
      const { data, error, response } = await api.POST('/api/v1/mentor/snapshot/load', {
        body: { name },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['mentor-snapshots'] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
      void qc.invalidateQueries({ queryKey: ['traffic'] });
    },
  });
}

// ---------- sesi + penilaian ----------

export function useMentorSessions(): UseQueryResult<MentorSessionInfo[], ApiError> {
  return useQuery({
    queryKey: ['mentor-sessions'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/mentor/sessions');
      if (data === undefined) throwProblem(error, response.status);
      return data.sessions;
    },
  });
}

export function useMentorSessionSummary(
  scenario: string | null,
): UseQueryResult<MentorSessionSummary, ApiError> {
  return useQuery({
    queryKey: ['mentor-session-summary', scenario],
    enabled: scenario !== null,
    queryFn: async () => {
      if (scenario === null) throw new Error('no scenario');
      const { data, error, response } = await api.GET('/api/v1/mentor/session-summary', {
        params: { query: { scenario } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useMentorAssessment(
  scenario: string | null,
): UseQueryResult<MentorAssessment, ApiError> {
  return useQuery({
    queryKey: ['mentor-assessment', scenario],
    enabled: scenario !== null,
    queryFn: async () => {
      if (scenario === null) throw new Error('no scenario');
      const { data, error, response } = await api.GET('/api/v1/mentor/assessment', {
        params: { query: { scenario } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useMentorAssessmentPut(): UseMutationResult<
  MentorAssessment,
  ApiError,
  {
    scenario: string;
    communicationScore?: number;
    communicationNote?: string;
    postmortemScore?: number;
    postmortemNote?: string;
  }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ scenario, ...body }) => {
      const { data, error, response } = await api.PUT('/api/v1/mentor/assessment', {
        params: { query: { scenario } },
        body,
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['mentor-assessment', vars.scenario] });
      void qc.invalidateQueries({ queryKey: ['mentor-session-summary', vars.scenario] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}
