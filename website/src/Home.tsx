import {GuideJaUrl, DiscordUrl} from './Const'
import React from 'react';
import { useIntl } from 'react-intl';

import image01 from './game.jpg';
import image02 from './flycast.png';
import image03 from './world.jpg';
import image04 from './stick.jpg';
import image05 from './headset.jpg';

import {
    Button,
    Col,
    Container,
    Jumbotron,
    Row,
    Image,
} from 'react-bootstrap';

export default function Home() {
    const intl = useIntl();
    return (
        <div className="main">
            <div className="Title title-bg image d-flex">
                <h1>gdxsv</h1>
                <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                        className={"join-btn mx-3 my-3 px-5 py-3"}>{intl.formatMessage({ id: "common.join" })}</Button>
            </div>

            <Container>
                <Row>
                    <Col>
                        <h2>{intl.formatMessage({ id: "home.revive.title" })}</h2>
                        <p>{intl.formatMessage({ id: "home.revive.description"}, { br: <br /> })}</p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image01} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>{intl.formatMessage({ id: "home.dreamcast.title" }, { br: <br /> })}</h3>
                        <p>{intl.formatMessage({ id: "home.dreamcast.description"}, { br: <br /> })}</p>
                        <p className={"gray-text"}>{intl.formatMessage({ id: "home.dreamcast.caption"}, { br: <br /> })}</p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image02} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>{intl.formatMessage({ id: "home.flycast.title"}, { br: <br /> })}</h3>
                        <p>{intl.formatMessage({
                            id: "home.flycast.description"},
                            {
                                br: <br />,
                                flycastGithub: <a href={"https://github.com/flyinghead/flycast"}>flycast</a>
                            })
                        }</p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image03} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>{intl.formatMessage({ id: "home.server.title"}, { br: <br /> })}</h3>
                        <p>{intl.formatMessage({ id: "home.server.description"}, { br: <br /> })}</p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image04} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>{intl.formatMessage({ id: "home.free.title"}, { br: <br /> })}</h3>
                        <p>{intl.formatMessage({ id: "home.free.description"}, { br: <br /> })}</p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image05} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>{intl.formatMessage({ id: "home.voice-chat.title"}, { br: <br /> })}</h3>
                        <p>{intl.formatMessage({ id: "home.voice-chat.description"}, { br: <br /> })}</p>
                    </Col>
                </Row>

                <Jumbotron fluid>
                    <Container>
                        <h2>{intl.formatMessage({ id: "common.start-now" })}</h2>
                        <div className={"d-flex justify-content-center"}>
                            <Button target="_blank" href={GuideJaUrl} variant={"outline-secondary"}
                                    className={"secondary join-btn mx-2 my-3 px-3 py-3"}>{intl.formatMessage({ id: "common.connection-guide" })}</Button>
                            <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                                    className={"join-btn mx-2 my-3 px-4 py-3"}>{intl.formatMessage({ id: "common.join" })}</Button>
                        </div>
                    </Container>
                </Jumbotron>
            </Container>
        </div>
    )
}
