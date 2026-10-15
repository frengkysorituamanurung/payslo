import {
  type InfiniteData,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from '@tanstack/react-query';
import {
  api,
  type AuditEntry,
  type BankStatus,
  type CreatePaymentRequest,
  type PaymentDetail,
  type PaymentList,
  type PaymentSummary,
  type ReleaseState,
  type RoutingConfig,
  type TrafficState,
} from '../api/client';

export class ApiError extends Error {
  readonly status: number;
  readonly detail: string;
  readonly fields: { field: string; message: string }[];

  constructor(status: number, detail: string, fields: { field: string; message: string }[] = []) {
    super(detail);
    this.name = 'ApiError';
    this.status = status;
    this.detail = detail;
    this.fields = fields;
  }
}

function throwProblem(error: unknown, status: number): never {
  const p = error as { detail?: string; errors?: { field: string; message: string }[] } | undefined;
  throw new ApiError(status, p?.detail ?? 'request failed', p?.errors ?? []);
}

export type PaymentFilters = {
  status?: NonNullable<PaymentList['items'][number]['status']>;
  provider?: 'bca' | 'mandiri' | 'ewallet';
  channel?: 'VA' | 'EWALLET' | 'QRIS';
};

const LIMIT = 25;

export function usePayments(filters: PaymentFilters) {
  return useInfiniteQuery<
    PaymentList,
    ApiError,
    InfiniteData<PaymentList>,
    readonly ['payments', PaymentFilters],
    string | undefined
  >({
    queryKey: ['payments', filters],
    initialPageParam: undefined,
    queryFn: async ({ pageParam }) => {
      const { data, error, response } = await api.GET('/api/v1/payments', {
        params: {
          query: {
            ...(filters.status === undefined ? {} : { status: filters.status }),
            ...(filters.provider === undefined ? {} : { provider: filters.provider }),
            ...(filters.channel === undefined ? {} : { channel: filters.channel }),
            limit: LIMIT,
            ...(pageParam === undefined ? {} : { cursor: pageParam }),
          },
        },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    getNextPageParam: (last) => last.nextCursor ?? undefined,
    refetchInterval: 5000,
  });
}

export function usePaymentDetail(id: string | null) {
  return useQuery<PaymentDetail>({
    queryKey: ['payment', id],
    enabled: id !== null,
    queryFn: async () => {
      if (id === null) throw new Error('no id');
      const { data, error, response } = await api.GET('/api/v1/payments/{id}', {
        params: { path: { id } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 5000,
  });
}

export function useBankStatus(id: string | null) {
  return useQuery<BankStatus>({
    queryKey: ['bank-status', id],
    enabled: false,
    retry: false,
    queryFn: async () => {
      if (id === null) throw new Error('no id');
      const { data, error, response } = await api.GET('/api/v1/payments/{id}/bank-status', {
        params: { path: { id } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
  });
}

export function useCreatePayment() {
  const qc = useQueryClient();
  return useMutation<PaymentSummary, ApiError, CreatePaymentRequest>({
    mutationFn: async (body) => {
      const { data, error, response } = await api.POST('/api/v1/payments', { body });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['payments'] });
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useRouting() {
  return useQuery<RoutingConfig>({
    queryKey: ['routing'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/routing');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 10_000,
  });
}

export function useSetRouting() {
  const qc = useQueryClient();
  return useMutation<RoutingConfig, ApiError, RoutingConfig>({
    mutationFn: async (body) => {
      const { data, error, response } = await api.PUT('/api/v1/routing', { body });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (data) => {
      qc.setQueryData(['routing'], data);
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useTraffic() {
  return useQuery<TrafficState>({
    queryKey: ['traffic'],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/traffic');
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    refetchInterval: 10_000,
  });
}

export function useSetTraffic() {
  const qc = useQueryClient();
  return useMutation<TrafficState, ApiError, number>({
    mutationFn: async (ratePerSec) => {
      const { data, error, response } = await api.PUT('/api/v1/traffic', {
        body: { ratePerSec, source: 'loadgen', checkedAt: new Date().toISOString() },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: (data) => {
      qc.setQueryData(['traffic'], data);
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useSetRelease() {
  const qc = useQueryClient();
  return useMutation<ReleaseState, ApiError, 'good' | 'bad'>({
    mutationFn: async (version) => {
      const { data, error, response } = await api.PUT('/api/v1/release', { body: { version } });
      if (data === undefined) throwProblem(error, response.status);
      return data;
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['audit'] });
    },
  });
}

export function useAudit(limit = 20) {
  return useQuery<AuditEntry[]>({
    queryKey: ['audit', limit],
    queryFn: async () => {
      const { data, error, response } = await api.GET('/api/v1/audit', {
        params: { query: { limit } },
      });
      if (data === undefined) throwProblem(error, response.status);
      return data.entries;
    },
    refetchInterval: 10_000,
  });
}
