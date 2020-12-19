import {GuideJaUrl, DiscordUrl} from './Const'
import React from 'react';
import {
    Button,
    Nav,
    Navbar,
} from 'react-bootstrap';
import { useIntl } from 'react-intl';

export default function Header() {
    const intl = useIntl();
    return (
        <Navbar id="page-header" expand="sm">
            <Navbar.Brand id="page-header-brand" href="#/">{intl.formatMessage({ id: "common.title" })}</Navbar.Brand>
            <Navbar.Toggle aria-controls="basic-navbar-nav"/>
            <Navbar.Collapse id="basic-navbar-nav">
                <Nav className="ml-auto">
                    <Nav.Link href="#/status">{intl.formatMessage({ id: "header.status" })}</Nav.Link>
                </Nav>
                <Button target="_blank" href={GuideJaUrl} variant={"outline-secondary"}
                        className={"join-btn mx-2 px-4 py-2"}>{intl.formatMessage({ id: "common.connection-guide" })}</Button>
                <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                        className={"join-btn mx-2 px-4 py-2"}><i className="fab fa-discord"></i>{intl.formatMessage({ id: "common.join" })}</Button>
            </Navbar.Collapse>
        </Navbar>
    );
}
