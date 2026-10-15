import { useTranslation } from 'react-i18next';
import { useParams } from 'react-router-dom';
import { MissionControl } from '../components/MissionControl';

const KEYS = [
    'mission',
    'map',
    'payments',
    'slo',
    'incidents',
    'scenarios',
    'control',
    'observe',
    'learn',
    'mentor',
    'settings',
] as const;

export function PlaceholderPage() {
    const { t } = useTranslation();
    const { page } = useParams();
    const key = KEYS.includes(page as (typeof KEYS)[number])
        ? (page as (typeof KEYS)[number])
        : 'mission';
    return (
        <section>
            <h1>{t(`nav.${key}`)}</h1>
            <p>{t('placeholder.body')}</p>
        </section>
    );
}

export function MissionPage() {
    return <MissionControl />;
}
