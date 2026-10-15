import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  api,
  type CapabilityMatrix,
  type SloApply,
  type SloDetail,
  type SloDrift,
  type SloFileInfo,
  type SloPlan,
  type SloSummary,
  type SloValidation,
} from '../api/client';
import { ApiError } from './usePayments';

export type {
  CapabilityMatrix,
  SloApply,
  SloDetail,
  SloDrift,
  SloFileInfo,
  SloPlan,
  SloSummary,
  SloValidation,
};

function throwProblem(error: unknown, status: number): never {
  const p = error as { detail?: string } | undefined;
  throw new ApiError(status, p?.detail ?? 'request failed');
}

export function useSlos(): UseQueryResult<{ slos: SloSummary[] }, ApiError> {
  return useQuery({
    queryKey: ['slos'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/slos');
      if (data === undefined) throwProblem(error, response.status);
      return { slos: data.slos };
    },
    refetchInterval: 10_000,
  });
}

export function useSloDetail(name: string | null): UseQueryResult<SloDetail, ApiError> {
  return useQuery({
    queryKey: ['slo', name],
    enabled: name !== null,
    queryFn: async () => {
      if (name === null) throw new Error('no name');
      const { data, error, response } = await api.GET('/api/v1/slos/{name}', {
        params: { path: { name } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 10_000,
  });
}

export function useSaveSlo(): UseMutationResult<
  SloFileInfo,
  ApiError,
  { name: string; content: string }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, content }) => {
      const { data, error, response } = await api.PUT('/api/v1/slos/{name}', {
        params: { path: { name } },
        body: { content },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['slos'] });
      void qc.invalidateQueries({ queryKey: ['slo', vars.name] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useValidateSlo(): UseMutationResult<
  SloValidation,
  ApiError,
  { name: string; content?: string }
> {
  return useMutation({
    mutationFn: async ({ name, content }) => {
      const { data, error, response } = await api.POST('/api/v1/slos/{name}/validate', {
        params: { path: { name } },
        body: content === undefined ? {} : { content },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function usePlanSlo(): UseMutationResult<SloPlan, ApiError, string> {
  return useMutation({
    mutationFn: async (name) => {
      const { data, error, response } = await api.POST('/api/v1/slos/{name}/plan', {
        params: { path: { name } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useApplySlo(): UseMutationResult<
  SloApply,
  ApiError,
  { name: string; connections?: string[] }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ name, connections }) => {
      const { data, error, response } = await api.POST('/api/v1/slos/{name}/apply', {
        params: { path: { name } },
        body: connections === undefined ? {} : { connections },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (_data, vars) => {
      void qc.invalidateQueries({ queryKey: ['slo', vars.name] });
      void qc.invalidateQueries({ queryKey: ['drift'] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useDrift(): UseQueryResult<SloDrift, ApiError> {
  return useQuery({
    queryKey: ['drift'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/drift');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 15_000,
  });
}

export function useCapabilities(): UseQueryResult<CapabilityMatrix, ApiError> {
  return useQuery({
    queryKey: ['capabilities'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/capabilities');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    staleTime: 60_000,
  });
}
