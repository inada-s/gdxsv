import axios from 'axios';
import React from 'react';
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
    battleCode: string
    disk: string
}

type ActiveGame = {
    battle_code: string
    region: string
    disk: string
    state: string
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
        lobby_users.sort(compByUserId);
        battle_users.sort(compByUserId);
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

        const renderOnlineUser = (u: OnlineUser) =>
            <tr key={u.user_id}>
                <td>{u.user_id}</td>
                <td>{u.name}</td>
                <td className={"text-center"}>
                    {u.team === "renpo" && (
                        <Image
                            src={renpoIcon}
                            style={{backgroundColor: "CornflowerBlue"}}
                            height="26" width="26"
                            roundedCircle/>
                    )}
                    {u.team === "zeon" && (
                        <Image
                            src={zeonIcon}
                            style={{backgroundColor: "mediumvioletred"}}
                            height="26" width="26"
                            roundedCircle/>
                    )}
                </td>
            </tr>

        const renderOnlineUserTable = (users: OnlineUser[]) =>
            <Table striped bordered hover size="sm">
                <thead>
                <tr>
                    <th>UserID</th>
                    <th>Name</th>
                    <th>Team</th>
                </tr>
                </thead>
                <tbody>
                {users.map(renderOnlineUser)}
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
                    <h3>Lobby {dc2_lobby_users.length} 人</h3>
                    {renderOnlineUserTable(dc2_lobby_users)}
                    <h3>Battle {dc2_battle_users.length} 人</h3>
                    {renderOnlineUserTable(dc2_battle_users)}
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
                    <h3>Lobby {dc1_lobby_users.length} 人</h3>
                    {renderOnlineUserTable(dc1_lobby_users)}
                    <h3>Battle {dc1_battle_users.length} 人</h3>
                    {renderOnlineUserTable(dc1_battle_users)}
                </Container>
            </Container>
        );
    }
}
