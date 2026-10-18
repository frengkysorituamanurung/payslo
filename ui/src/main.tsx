// Harus impor PERTAMA: mematikan probe `new Function` Zod sebelum modul lain
// membangun skema (lihat ui/src/zod-setup.ts).
import './zod-setup';
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { createBrowserRouter, RouterProvider } from 'react-router-dom';
import './i18n';
import './styles/tokens.css';
import { AppShell } from './components/AppShell';
import { ArchMap } from './components/ArchMap';
import {
    MissionPage,
    PlaceholderPage,
    MentorRoute,
    LearnRoute,
    ObserveRoute,
    SettingsRoute,
} from './pages/pages';
import { PaymentsPage } from './pages/Payments';
import { ControlPage } from './pages/Control';
import { SloPage } from './pages/Slo';
import { IncidentsPage } from './pages/Incidents';

const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: 1, refetchOnWindowFocus: false } },
});

const router = createBrowserRouter([
    {
        element: <AppShell />,
        children: [
            { index: true, element: <MissionPage /> },
            { path: 'map', element: <ArchMap /> },
            { path: 'payments', element: <PaymentsPage /> },
            { path: 'control', element: <ControlPage /> },
            { path: 'slo', element: <SloPage /> },
            { path: 'incidents', element: <IncidentsPage /> },
            { path: 'mentor', element: <MentorRoute /> },
            { path: 'learn', element: <LearnRoute /> },
            { path: 'observe', element: <ObserveRoute /> },
            { path: 'settings', element: <SettingsRoute /> },
            { path: ':page', element: <PlaceholderPage /> },
        ],
    },
]);

const root = document.getElementById('root');
if (root === null) throw new Error('root element missing');
createRoot(root).render(
    <StrictMode>
        <QueryClientProvider client={queryClient}>
            <RouterProvider router={router} />
        </QueryClientProvider>
    </StrictMode>,
);
