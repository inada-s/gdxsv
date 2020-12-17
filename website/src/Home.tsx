import {GuideJaUrl, DiscordUrl} from './Const'
import React from 'react';
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
    return (
        <div className="main">
            <div className="Title title-bg image d-flex">
                <h1>gdxsv</h1>
                <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                        className={"join-btn mx-3 my-3 px-5 py-3"}>参加する</Button>
            </div>

            <Container>
                <Row>
                    <Col>
                        <h2>今、連ジの通信対戦が蘇る！</h2>
                        <p>gdxsvは2001年に発売された「機動戦士ガンダム連邦vs.ジオンDX」の通信対戦を復活させるプロジェクトです。</p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image01} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>ドリキャス版</h3>
                        <p>
                            DC版を戦場に選びました。<br/>
                            DC版はアーケード版から多少修正が加えられていますが、アーケードプレイヤーにとっても大きな違和感なく楽しめるでしょう。
                        </p>
                        <p className={"gray-text"}>
                            『機動戦士ガンダム 連邦vs.ジオン & DX』<br/>
                            ©創通エージェンシー・サンライズ ©BANDAI 2001<br/>
                            ©CAPCOM CO.,LTD. 2001 ALL RIGHTS RESERVED
                        </p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image02} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>Powered by flycast</h3>
                        <p>
                            ドリームキャスト版の連ジを動かすのは、DCエミュレーター <a
                            href={"https://github.com/flyinghead/flycast"}>flycast</a> です。<br/>
                            gdxsvではエミュレーター内部に手を加えて通信遅延を最小化しました。
                        </p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image03} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>世界中にサーバーを用意しました</h3>
                        <p>
                            クラウドを活用して、対戦サーバーを世界中に用意しました。<br/>
                            地域固定のロビーもあれば、対戦相手と一番相性のよい対戦サーバーを自動的に検索して接続することもできます。
                        </p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image04} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>無料です</h3>
                        <p>
                            gdxsvはボランティアによって開発・運営されています。<br/>
                            対戦中1分13円の課金は発生しません。<br/>
                            ゲームソフトやPC・周辺機器は各自で準備する必要があります。
                        </p>
                    </Col>
                </Row>

                <Row className={"img-text my-5 align-items-center"}>
                    <Col sm={6}>
                        <Image src={image05} thumbnail={true}></Image>
                    </Col>
                    <Col sm={6}>
                        <h3>ボイスチャットで時間を共有しよう</h3>
                        <p>
                            無料アプリ Discord のサーバーを用意しました。<br/>
                            テキストチャットで対戦相手の募集したり、ボイスチャットで味方や対戦相手と通話しながら遊べます。スマートフォンでも通話に参加できます。
                        </p>
                    </Col>
                </Row>

                <Jumbotron fluid>
                    <Container>
                        <h2>作 戦 開 始</h2>
                        <div className={"d-flex justify-content-center"}>
                            <Button target="_blank" href={GuideJaUrl} variant={"outline-secondary"}
                                    className={"secondary join-btn mx-2 my-3 px-3 py-3"}>接続ガイド</Button>
                            <Button target="_blank" href={DiscordUrl} variant={"outline-primary"}
                                    className={"join-btn mx-2 my-3 px-4 py-3"}>参加する</Button>
                        </div>
                    </Container>
                </Jumbotron>
            </Container>
        </div>
    )
}
