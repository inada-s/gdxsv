import {GuideEnUrl, GuideJaUrl, DiscordUrl} from './Const'
import React from 'react';
import {
    Button,
    Nav,
    Navbar,
    NavDropdown
} from 'react-bootstrap';
import {useIntl} from 'react-intl';
import {LocaleType, ILocaleItem} from './Locale';

interface IHeader {
    locale: LocaleType;
    setLocale: React.Dispatch<React.SetStateAction<LocaleType>>;
    localeList: ILocaleItem[];
}

export default function Header({locale, setLocale, localeList}: IHeader) {
    const intl = useIntl();
    return (
        <Navbar id="page-header" expand="md">
            <Navbar.Brand id="page-header-brand" href="#/">{intl.formatMessage({id: "common.title"})}</Navbar.Brand>
            <Navbar.Toggle aria-controls="basic-navbar-nav"/>
            <Navbar.Collapse id="basic-navbar-nav">
                <Nav className="ml-auto">
                    <Nav.Link href="#/status">{intl.formatMessage({id: "header.status"})}</Nav.Link>
                </Nav>
                <Button target="_blank" href={locale === 'ja' ? GuideJaUrl : GuideEnUrl} variant={"outline-secondary"}
                        className={"join-btn mx-2 px-4 py-2"}>{intl.formatMessage({id: "common.connection-guide"})}</Button>
                <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                        className={"join-btn mx-2 px-4 py-2"}><i
                    className="fab fa-discord"></i>{intl.formatMessage({id: "common.join"})}</Button>
                <NavDropdown style={{fontSize: '1rem'}} title={(localeList.find(i => i.code === locale) || {}).name}
                             id="language-nav-dropdown">
                    {
                        localeList.map(({name, code}) => (
                            <NavDropdown.Item key={code} onClick={() => setLocale(code)}>{name}</NavDropdown.Item>
                        ))
                    }
                </NavDropdown>
            </Navbar.Collapse>
        </Navbar>
    );
}
