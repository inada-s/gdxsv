import React from 'react';
import { useIntl } from 'react-intl';
import { WebsiteUrl } from './Const';

interface ILink {
    href: string;
    ariaLabel: string;
    children: React.ReactNode;
}

const Link = ({ href, ariaLabel, children }: ILink) => (
    <a
        href={href}
        target="_blank"
        aria-label={ariaLabel}
        rel="noopener noreferrer nofollow"
        style={{ margin: '0 0.2rem' }}
    >
        {children}
    </a>
);

