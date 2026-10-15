import { useState } from 'react';
import { useTranslation } from 'react-i18next';

export function CopyCommand({ command }: { command: string }) {
    const { t } = useTranslation();
    const [copied, setCopied] = useState(false);
    return (
        <span
            style={{
                display: 'inline-flex',
                gap: 8,
                alignItems: 'center',
                background: 'var(--neutral-bg)',
                borderRadius: 'var(--radius)',
                padding: '6px 10px',
            }}
        >
            <code>{command}</code>
            <button
                type="button"
                aria-live="polite"
                onClick={() => {
                    void navigator.clipboard.writeText(command).then(() => {
                        setCopied(true);
                    });
                }}
            >
                {copied ? t('onboarding.copied') : t('onboarding.copy')}
            </button>
        </span>
    );
}
