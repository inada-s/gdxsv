import axios from 'axios';
import React, {useEffect} from 'react';

import {
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
        return (
            <Container>
                <h2>接続情報</h2>
                <h3>Lobby {this.state.lobby_users.length} 人</h3>
                <Table striped bordered hover size="sm">
                    <thead>
                    <tr>
                        <th>UserID</th>
                        <th>Name</th>
                        <th>Team</th>
                    </tr>
                    </thead>
                    <tbody>
                    {this.state.lobby_users.map((u: OnlineUser) => {
                        return <tr key={u.user_id}>
                            <td>{u.user_id}</td>
                            <td>{u.name}</td>
                            <td>{u.team}</td>
                        </tr>
                    })}
                    </tbody>
                </Table>

                <h3>Battle {this.state.battle_users.length} 人</h3>
                <Table striped bordered hover size="sm">
                    <thead>
                    <tr>
                        <th>UserID</th>
                        <th>Name</th>
                        <th>Team</th>
                    </tr>
                    </thead>
                    <tbody>
                    {this.state.battle_users.map((u: OnlineUser) => {
                        return <tr key={u.user_id}>
                            <td>{u.user_id}</td>
                            <td>{u.name}</td>
                            <td>{u.team}</td>
                        </tr>
                    })}
                    </tbody>
                </Table>
            </Container>
        );
    }
}
