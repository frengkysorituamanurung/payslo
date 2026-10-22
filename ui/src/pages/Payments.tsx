import { useState, type ReactNode, type SyntheticEvent } from 'react';
import { useTranslation } from 'react-i18next';
import type { PaymentSummary } from '../api/client';
import {
    ApiError,
    useBankStatus,
    useCreatePayment,
    usePaymentDetail,
    usePayments,
    type PaymentFilters,
} from '../hooks/usePayments';
import {
    formatIDR,
    formatIDRPlain,
    paymentFormSchema,
    uniqueOrderId,
    type PaymentForm,
} from '../utils/money';
import { formatLocal, formatRelative } from '../utils/time';
import { EmptyState, ErrorState } from '../components/States';
import { Timeline } from '../components/Timeline';
import { TableScroll } from '../components/TableScroll';

const STATUSES: ReadonlyArray<NonNullable<PaymentSummary['status']>> = [
    'CREATED',
    'PENDING',
    'SUCCESS',
    'FAILED',
    'EXPIRED',
];
const PROVIDERS = ['bca', 'mandiri', 'ewallet'] as const;
const CHANNELS = ['VA', 'EWALLET', 'QRIS'] as const;

const CHANNEL_LABEL: Record<(typeof CHANNELS)[number], string> = {
    VA: 'VA',
    EWALLET: 'EWALLET',
    QRIS: 'QRIS',
};

// Field error penuh: `message` adalah KODE dari skema Zod (bukan teks siap
// tampil) yang dipetakan ke i18n di placeCall (`form.*`). Tidak ada string
// bahasa hardcoded di komponen.
type FieldErrors = Record<string, string>;

function Field({
    id,
    label,
    error,
    hint,
    children,
}: {
    id: string;
    label: string;
    error?: string;
    hint?: string;
    children: ReactNode;
}) {
    return (
        <div className="field">
            <label htmlFor={id}>{label}</label>
            {children}
            {hint !== undefined && (
                <span style={{ display: 'block', fontSize: 12, color: 'var(--text-muted)' }}>
                    {hint}
                </span>
            )}
            {error !== undefined && (
                <span role="alert" style={{ display: 'block', color: 'var(--crit)' }}>
                    {error}
                </span>
            )}
        </div>
    );
}

function PaymentFormPanel() {
    const { t } = useTranslation();
    const [values, setValues] = useState<PaymentForm>(() => ({
        merchantId: 'merchant-demo',
        orderId: uniqueOrderId(),
        amount: '',
        channel: 'VA',
    }));
    const [errors, setErrors] = useState<FieldErrors>({});
    const [notice, setNotice] = useState<string | null>(null);
    const create = useCreatePayment();

    function fieldError(name: keyof PaymentForm, code: string): string {
        return name === 'amount' ? t(`form.amountErrors.${code}`) : t(`form.errors.${code}`);
    }

    function onSubmit(e: SyntheticEvent<HTMLFormElement>): void {
        e.preventDefault();
        setNotice(null);
        const parsed = paymentFormSchema.safeParse(values);
        if (!parsed.success) {
            const next: FieldErrors = {};
            for (const issue of parsed.error.issues) {
                const key = issue.path[0];
                if (typeof key === 'string') next[key] = issue.message;
            }
            setErrors(next);
            setNotice(t('form.invalid'));
            return;
        }
        setErrors({});
        create.mutate(
            {
                merchantId: parsed.data.merchantId,
                orderId: parsed.data.orderId,
                amount: parsed.data.amount,
                channel: parsed.data.channel,
            },
            {
                onSuccess: (sum) => {
                    setNotice(t('payments.created', { id: sum.id, order: sum.orderId }));
                    setValues((v) => ({ ...v, orderId: uniqueOrderId(), amount: '' }));
                },
                onError: (err) => {
                    if (err instanceof ApiError) {
                        const map: FieldErrors = {};
                        for (const f of err.fields) map[f.field] = f.message;
                        setErrors(map);
                    }
                    setNotice(null);
                },
            },
        );
    }

    return (
        <form
            className="panel"
            onSubmit={onSubmit}
            aria-labelledby="payments-form-title"
            noValidate
        >
            <h2 id="payments-form-title">{t('payments.formTitle')}</h2>
            <div
                style={{
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fit, minmax(min(180px, 100%), 1fr))',
                    gap: 12,
                }}
            >
                <Field
                    id="pf-merchant"
                    label={t('payments.merchantId')}
                    error={
                        errors['merchantId'] === undefined
                            ? undefined
                            : fieldError('merchantId', errors['merchantId'])
                    }
                >
                    <input
                        id="pf-merchant"
                        name="merchantId"
                        value={values.merchantId}
                        onChange={(e) => {
                            setValues((v) => ({ ...v, merchantId: e.target.value }));
                        }}
                        aria-invalid={errors['merchantId'] !== undefined}
                    />
                </Field>
                <Field
                    id="pf-order"
                    label={t('payments.orderId')}
                    hint={t('payments.autoOrderId')}
                    error={
                        errors['orderId'] === undefined
                            ? undefined
                            : fieldError('orderId', errors['orderId'])
                    }
                >
                    <input
                        id="pf-order"
                        name="orderId"
                        value={values.orderId}
                        onChange={(e) => {
                            setValues((v) => ({ ...v, orderId: e.target.value }));
                        }}
                        aria-invalid={errors['orderId'] !== undefined}
                    />
                </Field>
                <Field
                    id="pf-amount"
                    label={t('payments.amount')}
                    error={
                        errors['amount'] === undefined
                            ? undefined
                            : fieldError('amount', errors['amount'])
                    }
                >
                    <input
                        id="pf-amount"
                        name="amount"
                        inputMode="numeric"
                        value={values.amount}
                        onChange={(e) => {
                            setValues((v) => ({ ...v, amount: e.target.value }));
                        }}
                        aria-invalid={errors['amount'] !== undefined}
                    />
                </Field>
                <Field id="pf-channel" label={t('payments.channel')}>
                    <select
                        id="pf-channel"
                        name="channel"
                        value={values.channel}
                        onChange={(e) => {
                            setValues((v) => ({
                                ...v,
                                channel: e.target.value as PaymentForm['channel'],
                            }));
                        }}
                    >
                        {CHANNELS.map((c) => (
                            <option key={c} value={c}>
                                {CHANNEL_LABEL[c]}
                            </option>
                        ))}
                    </select>
                </Field>
            </div>
            <div style={{ marginTop: 12, display: 'flex', gap: 12, alignItems: 'center' }}>
                <button type="submit" disabled={create.isPending}>
                    {create.isPending ? t('payments.submitting') : t('payments.submit')}
                </button>
                {notice !== null && (
                    <span role="status" style={{ color: 'var(--ok)' }}>
                        {notice}
                    </span>
                )}
                {create.isError && (
                    <span role="alert" style={{ color: 'var(--crit)' }}>
                        {t('payments.createFailed')}: {create.error.detail}
                    </span>
                )}
            </div>
        </form>
    );
}

function PaymentDetailPanel({ id, onClose }: { id: string; onClose: () => void }) {
    const { t, i18n } = useTranslation();
    const detail = usePaymentDetail(id);
    const bank = useBankStatus(id);

    if (detail.isPending) return <p>{t('common.loading')}</p>;
    if (detail.isError)
        return (
            <ErrorState
                onRetry={() => {
                    void detail.refetch();
                }}
            />
        );
    const p = detail.data.payment;

    return (
        <section aria-labelledby="payment-detail-title" className="panel">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <h2 id="payment-detail-title" style={{ margin: 0 }}>
                    {t('payments.detail')}
                </h2>
                <button type="button" onClick={onClose}>
                    {t('common.close')}
                </button>
            </div>

            <dl
                style={{
                    display: 'grid',
                    gridTemplateColumns: 'auto 1fr',
                    gap: '4px 12px',
                    marginTop: 12,
                }}
            >
                <dt>{t('payments.orderId')}</dt>
                <dd>{p.orderId}</dd>
                <dt>{t('payments.merchantId')}</dt>
                <dd>{p.merchantId}</dd>
                <dt>{t('payments.channel')}</dt>
                <dd>{p.channel}</dd>
                <dt>{t('payments.provider')}</dt>
                <dd>{p.provider}</dd>
                <dt>{t('payments.amount')}</dt>
                <dd>{formatIDR(p.amount)}</dd>
                <dt>{t('payments.fee')}</dt>
                <dd>{formatIDR(p.fee)}</dd>
                <dt>{t('payments.status')}</dt>
                <dd>{p.status}</dd>
                <dt>{t('payments.providerRef')}</dt>
                <dd>{p.providerRef ?? t('payments.noRef')}</dd>
                <dt>{t('payments.createdAt')}</dt>
                <dd>
                    <time dateTime={p.createdAt} title={p.createdAt}>
                        {formatLocal(p.createdAt, i18n.language)}
                    </time>
                </dd>
            </dl>

            <p>
                <strong>
                    {t('payments.callbackReceived', { count: detail.data.callbackCount })}
                </strong>
                <br />
                <strong>
                    {t('payments.webhookDelivered', { count: detail.data.webhookCount })}
                </strong>
            </p>

            <h3>{t('payments.timeline')}</h3>
            <Timeline events={detail.data.timeline} />

            <h3>{t('payments.bankStatus')}</h3>
            <div style={{ display: 'flex', gap: 12, alignItems: 'center', flexWrap: 'wrap' }}>
                <button
                    type="button"
                    onClick={() => {
                        void bank.refetch();
                    }}
                    disabled={bank.isFetching}
                >
                    {bank.isFetching ? t('payments.checking') : t('payments.checkBank')}
                </button>
                {bank.data !== undefined && (
                    <span role="status">
                        {t('payments.bankSays', {
                            provider: bank.data.provider,
                            status: bank.data.status,
                            ref: bank.data.providerRef,
                        })}
                    </span>
                )}
                {bank.isError && (
                    <span role="alert" style={{ color: 'var(--crit)' }}>
                        {bank.error instanceof ApiError ? bank.error.detail : t('error.title')}
                    </span>
                )}
            </div>

            <h3>{t('payments.links')}</h3>
            {detail.data.links.log === null && detail.data.links.trace === null ? (
                <p style={{ color: 'var(--text-muted)' }}>{t('payments.linksInactive')}</p>
            ) : (
                <ul>
                    {detail.data.links.log !== null && (
                        <li>
                            <a href={detail.data.links.log} target="_blank" rel="noreferrer">
                                {t('payments.logLink')}
                            </a>
                        </li>
                    )}
                    {detail.data.links.trace !== null && (
                        <li>
                            <a href={detail.data.links.trace} target="_blank" rel="noreferrer">
                                {t('payments.traceLink')}
                            </a>
                        </li>
                    )}
                </ul>
            )}
        </section>
    );
}

export function PaymentsPage() {
    const { t, i18n } = useTranslation();
    const [filters, setFilters] = useState<PaymentFilters>({});
    const [selected, setSelected] = useState<string | null>(null);
    const list = usePayments(filters);

    const items = list.data?.pages.flatMap((page) => page.items) ?? [];

    return (
        <section className="page" aria-labelledby="payments-title">
            <h1 id="payments-title">{t('nav.payments')}</h1>

            <PaymentFormPanel />

            <section className="panel" aria-labelledby="payments-table-title">
                <h2 id="payments-table-title">{t('payments.table')}</h2>
                <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 12 }}>
                    <label>
                        {t('payments.filterStatus')}
                        <select
                            value={filters.status ?? ''}
                            onChange={(e) => {
                                const v = e.target.value;
                                setFilters((f) =>
                                    v === ''
                                        ? { ...f, status: undefined }
                                        : {
                                              ...f,
                                              status: v as NonNullable<PaymentSummary['status']>,
                                          },
                                );
                            }}
                        >
                            <option value="">{t('payments.allStatus')}</option>
                            {STATUSES.map((s) => (
                                <option key={s} value={s}>
                                    {s}
                                </option>
                            ))}
                        </select>
                    </label>
                    <label>
                        {t('payments.filterProvider')}
                        <select
                            value={filters.provider ?? ''}
                            onChange={(e) => {
                                const v = e.target.value;
                                setFilters((f) =>
                                    v === ''
                                        ? { ...f, provider: undefined }
                                        : { ...f, provider: v as (typeof PROVIDERS)[number] },
                                );
                            }}
                        >
                            <option value="">{t('payments.allProvider')}</option>
                            {PROVIDERS.map((p) => (
                                <option key={p} value={p}>
                                    {p}
                                </option>
                            ))}
                        </select>
                    </label>
                    <label>
                        {t('payments.filterChannel')}
                        <select
                            value={filters.channel ?? ''}
                            onChange={(e) => {
                                const v = e.target.value;
                                setFilters((f) =>
                                    v === ''
                                        ? { ...f, channel: undefined }
                                        : { ...f, channel: v as (typeof CHANNELS)[number] },
                                );
                            }}
                        >
                            <option value="">{t('payments.allChannel')}</option>
                            {CHANNELS.map((c) => (
                                <option key={c} value={c}>
                                    {CHANNEL_LABEL[c]}
                                </option>
                            ))}
                        </select>
                    </label>
                </div>

                {list.isPending && <p>{t('common.loading')}</p>}
                {list.isError && (
                    <ErrorState
                        onRetry={() => {
                            void list.refetch();
                        }}
                    />
                )}
                {list.isSuccess && items.length === 0 && (
                    <EmptyState next={t('payments.emptyNext')} />
                )}
                {items.length > 0 && (
                    <TableScroll label={t('payments.table')}>
                        <table style={{ borderCollapse: 'collapse', width: '100%' }}>
                            <caption style={{ textAlign: 'start', color: 'var(--text-muted)' }}>
                                {t('payments.tableCaption')}
                            </caption>
                            <thead>
                                <tr>
                                    <th scope="col">{t('payments.colTime')}</th>
                                    <th scope="col">{t('payments.colOrder')}</th>
                                    <th scope="col">{t('payments.colChannel')}</th>
                                    <th scope="col">{t('payments.colProvider')}</th>
                                    <th scope="col">{t('payments.colAmount')}</th>
                                    <th scope="col">{t('payments.colStatus')}</th>
                                    <th scope="col">{t('payments.colCallback')}</th>
                                    <th scope="col">{t('payments.colAction')}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {items.map((p) => (
                                    <tr key={p.id}>
                                        <td>
                                            <time dateTime={p.createdAt} title={p.createdAt}>
                                                {formatRelative(p.createdAt, i18n.language)}
                                            </time>
                                        </td>
                                        <td>{p.orderId}</td>
                                        <td>{p.channel}</td>
                                        <td>{p.provider}</td>
                                        <td style={{ textAlign: 'end' }}>
                                            {formatIDRPlain(p.amount)}
                                        </td>
                                        <td>{p.status}</td>
                                        <td>
                                            {p.duplicateCallback ? (
                                                <span role="note">
                                                    {t('payments.callbackReceived', {
                                                        count: p.callbackCount,
                                                    })}
                                                </span>
                                            ) : (
                                                String(p.callbackCount)
                                            )}
                                        </td>
                                        <td>
                                            <button
                                                type="button"
                                                onClick={() => {
                                                    setSelected(p.id);
                                                }}
                                            >
                                                {t('payments.viewDetail')}
                                            </button>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    </TableScroll>
                )}

                {list.hasNextPage && (
                    <div style={{ marginTop: 12 }}>
                        <button
                            type="button"
                            onClick={() => {
                                void list.fetchNextPage();
                            }}
                            disabled={list.isFetchingNextPage}
                        >
                            {list.isFetchingNextPage ? t('common.loading') : t('payments.loadMore')}
                        </button>
                    </div>
                )}
            </section>

            {selected !== null && (
                <div style={{ marginTop: 16 }}>
                    <PaymentDetailPanel
                        id={selected}
                        onClose={() => {
                            setSelected(null);
                        }}
                    />
                </div>
            )}
        </section>
    );
}
