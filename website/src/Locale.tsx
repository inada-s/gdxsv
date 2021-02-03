import { useState } from 'react';
import ja from './translations/ja.json';
import zh from './translations/zh.json';
import en from './translations/en.json';

const browserLanguage:string = (navigator.languages && navigator.languages[0]) || navigator.language;

export type LocaleType = "ja" | "zh" | "en";
export interface ILocaleItem {
    name: string;
    code: LocaleType;
}

// seems typescript bug here, use any for now
// export default ():[string, React.Dispatch<React.SetStateAction<LocaleType>>, ILocaleItem[], any] => {
export default ():any[] => {
    const defaultLocale:LocaleType =
        localStorage['locale'] ||
        (browserLanguage && browserLanguage.toLowerCase().split(/[_-]+/)[0]) || // Remove the region code
        'ja';
    
    const localeList:ILocaleItem[] = [
        { name: '日本語', code: 'ja' },
        { name: '中文', code: 'zh' },
        { name: 'English', code: 'en' }
    ];
    const [locale, setLocale] = useState(
        localeList.map(item => item.code).includes(defaultLocale) ?
        defaultLocale :
        'ja'
    );
    const messages = { ja: ja, zh: zh, en: en };

    const changeLocale = (selectedLocale: LocaleType) => {
        setLocale(selectedLocale);
        localStorage.setItem('locale',selectedLocale)
    }
    
    return [locale, changeLocale, localeList, messages[locale]];
};
