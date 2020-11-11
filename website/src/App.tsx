import React from 'react';
import {
    HashRouter as Router,
    Switch,
    Route,
} from "react-router-dom";

import 'bootstrap/dist/css/bootstrap.min.css';
import './App.css';
import Header from './Header';
import Home from './Home';
import Status from './Status';
import Footer from './Footer';

function App() {
    console.log('public url: ', process.env.PUBLIC_URL)
    return (
        <React.Fragment>
            <Header/>
            <Router basename={process.env.PUBLIC_URL}>
                <div>
                    <Switch>
                        <Route exact path="/">
                            <Home/>
                        </Route>
                        <Route path="/status">
                            <Status/>
                        </Route>
                    </Switch>
                </div>
            </Router>
            <Footer/>
        </React.Fragment>
    );
}

export default App;
