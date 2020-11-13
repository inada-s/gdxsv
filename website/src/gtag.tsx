export const GA_MEASUREMENT_ID = 'G-FJN2KR1FWT';

export const pageview = (url: URL | string) => {
    window.gtag("config", GA_MEASUREMENT_ID, {
        page_path: url
    });
};

type GTagEvent = {
    action: string;
    category: string;
    label: string;
    value: number;
};

export const event = ({action, category, label, value}: GTagEvent) => {
    window.gtag("event", action, {
        event_category: category,
        event_label: label,
        value: value
    });
};