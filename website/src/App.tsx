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
import {pageview} from './gtag'


function App() {
    return (
        <React.Fragment>
            <Header/>
            <Router basename={process.env.PUBLIC_URL}>
                <div>
                    <Switch>
                        <Route exact path="/">
                            {pageview('/')}
                            <Home/>
                        </Route>
                        <Route path="/status">
                            {pageview('/status')}
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
