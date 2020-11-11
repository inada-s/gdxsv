import {GuideJaUrl, DiscordUrl} from './Const'
import React from 'react';
import {
    Button,
    Nav,
    Navbar,
} from 'react-bootstrap';

export default function Header() {
    return (
        <Navbar id="page-header" expand="sm">
            <Navbar.Brand id="page-header-brand" href="#/">gdxsv</Navbar.Brand>
            <Navbar.Toggle aria-controls="basic-navbar-nav"/>
            <Navbar.Collapse id="basic-navbar-nav">
                <Nav className="ml-auto">
                    <Nav.Link href="#/status">接続情報</Nav.Link>
                </Nav>
                <Button target="_blank" href={GuideJaUrl} variant={"outline-secondary"}
                        className={"join-btn mx-2 px-4 py-2"}>接続ガイド</Button>
                <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                        className={"join-btn mx-2 px-4 py-2"}><i className="fab fa-discord"></i>参加する</Button>
            </Navbar.Collapse>
        </Navbar>
    );
}
