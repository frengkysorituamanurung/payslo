import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  api,
  type Approval,
  type Incident,
  type IncidentDetail,
  type LabSettings,
  type Postmortem,
  type RunbookStep,
  type StepOutput,
} from '../api/client';
import { ApiError } from './usePayments';

export type {
  Approval,
  Incident,
  IncidentDetail,
  LabSettings,
  Postmortem,
  RunbookStep,
  StepOutput,
};

function throwProblem(error: unknown, status: number): never {
  const p = error as { detail?: string; error?: string } | undefined;
  throw new ApiError(status, p?.detail ?? p?.error ?? 'request failed');
}

export function useIncidents(
  status?: 'open' | 'acknowledged' | 'resolved',
): UseQueryResult<Incident[], ApiError> {
  return useQuery({
    queryKey: ['incidents', status ?? ''],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/incidents', {
        params: { query: status === undefined ? {} : { status } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data.incidents;
    },
    refetchInterval: 10_000,
  });
}

export function useIncidentDetail(id: string | null): UseQueryResult<IncidentDetail, ApiError> {
  return useQuery({
    queryKey: ['incident', id],
    enabled: id !== null,
    queryFn: async () => {
      if (id === null) throw new Error('no id');
      const { data, error, response } = await api.GET('/api/v1/incidents/{id}', {
        params: { path: { id } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 10_000,
  });
}

export function useAddNote(): UseMutationResult<unknown, ApiError, { id: string; text: string }> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, text }) => {
      const { data, error, response } = await api.POST('/api/v1/incidents/{id}/notes', {
        params: { path: { id } },
        body: { text },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['incident', vars.id] });
    },
  });
}

export function useRequestApproval(): UseMutationResult<
  Approval,
  ApiError,
  { id: string; action: string; step: number }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, action, step }) => {
      const { data, error, response } = await api.POST('/api/v1/incidents/{id}/approvals', {
        params: { path: { id } },
        body: { action, step },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data.approval;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['incident', vars.id] });
      void qc.invalidateQueries({ queryKey: ['approvals'] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useDecideApproval(): UseMutationResult<
  Approval,
  ApiError,
  {
    aid: string;
    incidentId: string;
    decision: 'approved' | 'rejected';
    by: string;
    reason?: string;
  }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ aid, decision, by, reason }) => {
      const { data, error, response } = await api.POST('/api/v1/approvals/{aid}/decision', {
        params: { path: { aid } },
        body: reason === undefined ? { decision, by } : { decision, by, reason },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data.approval;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['incident', vars.incidentId] });
      void qc.invalidateQueries({ queryKey: ['approvals'] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useRunStep(): UseMutationResult<StepOutput, ApiError, { id: string; idx: number }> {
  return useMutation({
    mutationFn: async ({ id, idx }) => {
      const { data, error, response } = await api.POST(
        '/api/v1/incidents/{id}/runbook/steps/{idx}/run',
        { params: { path: { id, idx } } },
      );
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function usePendingApprovals(): UseQueryResult<Approval[], ApiError> {
  return useQuery({
    queryKey: ['approvals', 'pending'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/approvals', {
        params: { query: { status: 'pending' } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data.approvals;
    },
    refetchInterval: 10_000,
  });
}

export function usePostmortem(id: string | null): UseQueryResult<Postmortem, ApiError> {
  return useQuery({
    queryKey: ['postmortem', id],
    enabled: id !== null,
    queryFn: async () => {
      if (id === null) throw new Error('no id');
      const { data, error, response } = await api.GET('/api/v1/incidents/{id}/postmortem', {
        params: { path: { id } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useSavePostmortem(): UseMutationResult<
  Postmortem,
  ApiError,
  { id: string; status: 'draft' | 'published'; sections: Postmortem['sections'] }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ id, status, sections }) => {
      const { data, error, response } = await api.PUT('/api/v1/incidents/{id}/postmortem', {
        params: { path: { id } },
        body: { status, sections },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['postmortem', vars.id] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useSettings(): UseQueryResult<LabSettings, ApiError> {
  return useQuery({
    queryKey: ['settings'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/settings');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    staleTime: 60_000,
  });
}

export function useSaveSettings(): UseMutationResult<
  LabSettings,
  ApiError,
  { traineeName?: string; lowSampleThreshold?: number }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (body) => {
      const { data, error, response } = await api.PUT('/api/v1/settings', { body });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (data) => {
      qc.setQueryData(['settings'], data);
    },
  });
}
