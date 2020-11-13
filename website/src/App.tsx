import React, { useRef, useEffect } from 'react';
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
    const router = useRef(null);

    useEffect(() => {
        // @ts-ignore
        router.current.history.listen((location) => {
            // @ts-ignore
            window.gtag('config', 'G-FJN2KR1FWT', {
                'page_path': `${location.pathname}${location.search}`
            });
        });
    });

    return (
        <React.Fragment>
            <Header/>
            <Router basename={process.env.PUBLIC_URL} ref={router}>
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
