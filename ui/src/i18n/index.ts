import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import en from './locales/en.json';
import id from './locales/id.json';

// U8 (WCAG 3.1.1 Language of Page): sinkronkan atribut `lang` dokumen dengan
// bahasa aktif, termasuk saat trainee mengganti bahasa di topbar. Pembaca
// layar memakai ini untuk memilih pelafalan yang benar.
i18n.on('languageChanged', (lng) => {
  if (typeof document !== 'undefined') document.documentElement.lang = lng;
});

void i18n.use(initReactI18next).init({
  resources: { id: { translation: id }, en: { translation: en } },
  lng: 'id',
  fallbackLng: 'id',
  interpolation: { escapeValue: true },
});

export default i18n;
