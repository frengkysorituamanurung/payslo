import * as Dialog from '@radix-ui/react-dialog';
import { useTranslation } from 'react-i18next';

// U4: aksi berisiko = konfirmasi eksplisit (ketik nama target).
export function ConfirmDialog({
    target,
    open,
    onOpenChange,
    onConfirm,
}: {
    target: string;
    open: boolean;
    onOpenChange: (open: boolean) => void;
    onConfirm: () => void;
}) {
    const { t } = useTranslation();
    return (
        <Dialog.Root open={open} onOpenChange={onOpenChange}>
            <Dialog.Portal>
                <Dialog.Overlay
                    style={{ position: 'fixed', inset: 0, background: 'rgba(0,0,0,0.5)' }}
                />
                <Dialog.Content
                    aria-describedby={undefined}
                    style={{
                        position: 'fixed',
                        top: '30%',
                        left: '50%',
                        transform: 'translateX(-50%)',
                        background: 'var(--surface)',
                        border: '1px solid var(--border)',
                        borderRadius: 'var(--radius)',
                        padding: 16,
                    }}
                >
                    <Dialog.Title>{t('common.confirm')}</Dialog.Title>
                    <form
                        onSubmit={(e) => {
                            e.preventDefault();
                            const v = new FormData(e.currentTarget).get('ack');
                            if (v === target) {
                                onConfirm();
                                onOpenChange(false);
                            }
                        }}
                    >
                        <label>
                            {target}
                            <input
                                name="ack"
                                autoComplete="off"
                                style={{ display: 'block', marginTop: 8 }}
                            />
                        </label>
                        <div style={{ marginTop: 12, display: 'flex', gap: 8 }}>
                            <button type="submit">{t('common.confirm')}</button>
                            <Dialog.Close>{t('common.cancel')}</Dialog.Close>
                        </div>
                    </form>
                </Dialog.Content>
            </Dialog.Portal>
        </Dialog.Root>
    );
}
