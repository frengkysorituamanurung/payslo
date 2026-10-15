import { useEffect, useRef, useState } from 'react';
import type { MetricsSummary } from './useLab';

export type LiveState = 'live' | 'reconnecting' | 'polling';

// Klien SSE U1: reconnect otomatis (EventSource), indikator live/terputus,
// fallback polling bila SSE gagal 2x beruntun, throttle render 1/detik.
export type IncidentMini = { id: string; title: string; severity: string };
export type ApprovalMini = { id: string; incidentId: string; action: string };

// Klien SSE U1: reconnect otomatis (EventSource), indikator live/terputus,
// fallback polling bila SSE gagal 2x beruntun, throttle render 1/detik.
// U4: event incident (insiden terbuka) + approval (menunggu keputusan).
export function useEvents(demo?: string): {
  live: LiveState;
  metrics: MetricsSummary | null;
  labUp: boolean | null;
  openIncidents: IncidentMini[];
  pendingApprovals: ApprovalMini[];
} {
  const [live, setLive] = useState<LiveState>('reconnecting');
  const [metrics, setMetrics] = useState<MetricsSummary | null>(null);
  const [labUp, setLabUp] = useState<boolean | null>(null);
  const [openIncidents, setOpenIncidents] = useState<IncidentMini[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<ApprovalMini[]>([]);
  const fails = useRef(0);
  const lastPaint = useRef(0);
  const pollTimer = useRef<ReturnType<typeof setInterval> | null>(null);

  useEffect(() => {
    let dead = false;
    let es: EventSource | null = null;

    const paint = (m: MetricsSummary | null, up: boolean | null) => {
      // Throttle: maks 1 render/detik.
      const now = Date.now();
      if (now - lastPaint.current < 1000) return;
      lastPaint.current = now;
      if (m !== null) setMetrics(m);
      if (up !== null) setLabUp(up);
    };

    const startPolling = () => {
      if (dead || pollTimer.current !== null) return;
      setLive('polling');
      const tick = () => {
        void fetch(`/api/v1/metrics/summary${demo === undefined ? '' : `?demo=${demo}`}`)
          .then((r) => (r.ok ? r.json() : null))
          .then((j: MetricsSummary | null) => {
            if (!dead && j !== null) {
              setMetrics(j);
              lastPaint.current = Date.now();
            }
          })
          .catch(() => {});
      };
      tick();
      pollTimer.current = setInterval(tick, 5000);
    };

    const connect = () => {
      if (dead) return;
      try {
        es = new EventSource(`/api/v1/events${demo === undefined ? '' : `?demo=${demo}`}`);
      } catch {
        startPolling();
        return;
      }
      es.addEventListener('metrics', (ev: Event) => {
        const raw: unknown = (ev as MessageEvent).data;
        if (typeof raw !== 'string') return;
        try {
          const j = JSON.parse(raw) as MetricsSummary;
          fails.current = 0;
          setLive('live');
          paint(j, null);
        } catch {
          // abaikan frame rusak, tunggu berikutnya
        }
      });
      es.addEventListener('health', (ev: Event) => {
        const raw: unknown = (ev as MessageEvent).data;
        if (typeof raw !== 'string') return;
        try {
          const j = JSON.parse(raw) as { labUp: boolean };
          setLabUp(j.labUp);
        } catch {
          // abaikan
        }
      });
      es.addEventListener('incident', (ev: Event) => {
        const raw: unknown = (ev as MessageEvent).data;
        if (typeof raw !== 'string') return;
        try {
          const j = JSON.parse(raw) as { open: IncidentMini[] };
          if (Array.isArray(j.open)) {
            fails.current = 0;
            setLive('live');
            setOpenIncidents(j.open);
          }
        } catch {
          // abaikan frame rusak, tunggu berikutnya
        }
      });
      es.addEventListener('approval', (ev: Event) => {
        const raw: unknown = (ev as MessageEvent).data;
        if (typeof raw !== 'string') return;
        try {
          const j = JSON.parse(raw) as { pending: ApprovalMini[] };
          if (Array.isArray(j.pending)) {
            fails.current = 0;
            setLive('live');
            setPendingApprovals(j.pending);
          }
        } catch {
          // abaikan frame rusak, tunggu berikutnya
        }
      });
      es.onerror = () => {
        fails.current += 1;
        setLive('reconnecting');
        if (fails.current >= 2) {
          es?.close();
          startPolling();
        }
      };
    };

    connect();
    return () => {
      dead = true;
      es?.close();
      if (pollTimer.current !== null) clearInterval(pollTimer.current);
    };
  }, [demo]);

  return { live, metrics, labUp, openIncidents, pendingApprovals };
}
