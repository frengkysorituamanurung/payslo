import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseMutationResult,
  type UseQueryResult,
} from '@tanstack/react-query';
import {
  api,
  type ChecklistItem,
  type CurriculumModule,
  type GlossaryTerm,
  type LearnProgressItem,
} from '../api/client';
import { ApiError } from './usePayments';

export type { ChecklistItem, CurriculumModule, GlossaryTerm, LearnProgressItem };

function throwProblem(error: unknown, status: number): never {
  const p = error as { detail?: string; error?: string } | undefined;
  throw new ApiError(status, p?.detail ?? p?.error ?? 'request failed');
}

// ---------- kurikulum ----------

export function useCurriculum(): UseQueryResult<CurriculumModule[], ApiError> {
  return useQuery({
    queryKey: ['curriculum'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/learn/curriculum');
      if (data === undefined) throwProblem(error, response.status);
      return data.modules;
    },
  });
}

// ---------- glosarium ----------

export function useGlossary(): UseQueryResult<GlossaryTerm[], ApiError> {
  return useQuery({
    queryKey: ['glossary'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/learn/glossary');
      if (data === undefined) throwProblem(error, response.status);
      return data.terms;
    },
  });
}

// ---------- progres ----------

export function useLearnProgress(): UseQueryResult<LearnProgressItem[], ApiError> {
  return useQuery({
    queryKey: ['learn-progress'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/learn/progress');
      if (data === undefined) throwProblem(error, response.status);
      return data.items;
    },
  });
}

export function useLearnProgressPut(): UseMutationResult<
  { items: LearnProgressItem[] },
  ApiError,
  { module: string; key: string; done: boolean }
> {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ module, key, done }) => {
      const { data, error, response } = await api.PUT('/api/v1/learn/progress', {
        body: { module, key, done },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['learn-progress'] });
    },
  });
}
