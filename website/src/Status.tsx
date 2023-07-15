import axios from 'axios';
import React from 'react';
import { FormattedMessage } from "react-intl";

import renpoIcon from './renpo.png'
import zeonIcon from './zeon.png'
import disk1Icon from './renji1.png'
import disk2Icon from './renji2.png'

import {
    Image,
    Container,
    Table,
} from 'react-bootstrap';

type OnlineUser = {
    user_id: string
    name: string
    team: string
    lobby_id: number
    battle_code: string
    battle_pos: number
    disk: string
    flycast: string
    platform: string

    users_in_battle: number
}

type ActiveGame = {
    battle_code: string
    region: string
    disk: string
    state: string
    lobby_id: number
    updated_at: Date
}

type Props = {}

type State = {
    interval: any
    lobby_users: OnlineUser[]
    battle_users: OnlineUser[]
    active_games: ActiveGame[]
}

export default class Status extends React.Component<Props, State> {
    constructor(props: Props) {
        super(props);

        this.state = {
            interval: null,
            lobby_users: [],
            battle_users: [],
            active_games: [],
        }
    }

    async updateLbsStatus() {
        const resp = await axios.get("https://asia-northeast1-gdxsv-274515.cloudfunctions.net/lbsapi/status")
        const lobby_users = resp.data.lobby_users || []
        const battle_users = resp.data.battle_users || []
        const active_games = resp.data.active_games || []
        const compByUserId = (a: OnlineUser, b: OnlineUser) => {
            if (a.user_id < b.user_id) return -1;
            if (a.user_id > b.user_id) return 1;
            return 0;
        };
        const compByBattleCode = (a: OnlineUser, b: OnlineUser) => {
            if (a.battle_code < b.battle_code) return -1;
            if (a.battle_code > b.battle_code) return 1;
            if (a.battle_pos < b.battle_pos) return -1;
            if (a.battle_pos > b.battle_pos) return 1;
            return 0;
        };
        lobby_users.sort(compByUserId);
        battle_users.sort(compByBattleCode);

        this.setState({
            lobby_users,
            battle_users,
            active_games,
        });
    }

    async componentDidMount() {
        const self = this;
        const interval = setInterval(async function () {
            await self.updateLbsStatus();
        }, 1000 * 30);
        this.setState({interval})
        await this.updateLbsStatus();
    }

    componentWillUnmount() {
        console.log("clear interval");
        clearInterval(this.state.interval);
    }

    render() {
        const dc1_lobby_users = this.state.lobby_users.filter((u: OnlineUser) => u.disk === "dc1");
        const dc1_battle_users = this.state.battle_users.filter((u: OnlineUser) => u.disk === "dc1");
        const dc2_lobby_users = this.state.lobby_users.filter((u: OnlineUser) => u.disk === "dc2");
        const dc2_battle_users = this.state.battle_users.filter((u: OnlineUser) => u.disk === "dc2");

        const renderTeamIcon = (team: string) => <>
            {team === "renpo" && (
                <Image
                    className={"ml-2 mr-2 mb-2"}
                    src={renpoIcon}
                    style={{backgroundColor: "CornflowerBlue"}}
                    height="20" width="20"
                    roundedCircle/>
            )}
            {team === "zeon" && (
                <Image
                    className={"ml-2 mr-2 mb-2"}
                    src={zeonIcon}
                    style={{backgroundColor: "mediumvioletred"}}
                    height="20" width="20"
                    roundedCircle/>
            )}
            {team === "" && (
                <Image
                    className={"ml-2 mr-2 mb-2"}
                    src={renpoIcon}
                    style={{backgroundColor: "CornflowerBlue", opacity: "0"}}
                    height="20" width="20"
                    roundedCircle/>
            )}
        </>

        const renderOnlineUser = (u: OnlineUser) =>
            <tr key={u.user_id}>
                <td>
                    {renderTeamIcon(u.team)}
                    <span className={"user-id m-2"}>{u.user_id}</span>
                    <span className={"user-name m-2"}>{u.name}</span>
                    <span className={"badge m-1 float-right"}>{u.flycast}</span>
                </td>
                <td className={"text-center align-middle"} >
                    <FormattedMessage id={"game.lobby" + u.lobby_id} />
                </td>
            </tr>

        const renderOnlineUserTable = (users: OnlineUser[]) =>
            <Table striped bordered hover size="sm">
                <thead>
                <tr>
                    <th className={"text-center"}><FormattedMessage id="status.user" /></th>
                    <th className={"text-center"} style={{width: "20%"}}><FormattedMessage id="status.user-place" /></th>
                </tr>
                </thead>
                <tbody>
                {users.map(renderOnlineUser)}
                </tbody>
            </Table>

        const renderBattleUser = (u: OnlineUser, idx: number, arr: OnlineUser[]) =>
            <tr key={u.user_id}>
                <td>
                    {renderTeamIcon(u.team)}
                    <span className={"user-id m-2"}>{u.user_id}</span>
                    <span className={"user-name m-2"}>{u.name}</span>
                    <span className={"badge m-1 float-right"}>{u.flycast}</span>
                </td>
                {u.battle_pos === 1 && (
                    <td className={"text-center align-middle"} rowSpan={
                        arr.filter(o => u.battle_code === o.battle_code)
                           .map(o => o.battle_pos)
                           .reduce((prev, curr) => prev < curr ? curr : prev)
                    }>
                        <FormattedMessage id={"game.lobby" + u.lobby_id} /> <br/>
                    </td>
                )}
            </tr>

        const renderBattleUserTable = (users: OnlineUser[]) =>
            <Table striped bordered hover size="sm">
                <thead>
                <tr>
                    <th className={"text-center"}><FormattedMessage id="status.user" /></th>
                    <th className={"text-center"} style={{width: "20%"}}><FormattedMessage id="status.user-place" /></th>
                </tr>
                </thead>
                <tbody>
                {users.map(renderBattleUser)}
                </tbody>
            </Table>

        return (
            <Container>
                <Container>
                    <div className={"text-center mt-3"}>
                        <Image
                            src={disk2Icon}
                            style={{backgroundColor: "black"}}
                            height="60"
                            rounded
                        />
                    </div>
                    <h3><FormattedMessage id="status.lobby" values={{ peopleCount: dc2_lobby_users.length }} /></h3>
                    {renderOnlineUserTable(dc2_lobby_users)}
                    <h3><FormattedMessage id="status.battle" values={{ peopleCount: dc2_battle_users.length }} /></h3>
                    {renderBattleUserTable(dc2_battle_users)}
                </Container>

                <Container>
                    <div className={"text-center mt-5"}>
                        <Image
                            src={disk1Icon}
                            style={{backgroundColor: "black"}}
                            height="60"
                            rounded
                        />
                    </div>
                    <h3><FormattedMessage id="status.lobby" values={{ peopleCount: dc1_lobby_users.length }} /></h3>
                    {renderOnlineUserTable(dc1_lobby_users)}
                    <h3><FormattedMessage id="status.battle" values={{ peopleCount: dc1_battle_users.length }} /></h3>
                    {renderBattleUserTable(dc1_battle_users)}
                </Container>
            </Container>
        );
    }
}
