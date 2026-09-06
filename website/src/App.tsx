import React from 'react';
import {
    HashRouter as Router,
    Routes,
    Route,
} from "react-router-dom";
import { IntlProvider } from 'react-intl';
import useLocale from './Locale';

import 'bootstrap/dist/css/bootstrap.min.css';
import './App.css';
import Header from './Header';
import Home from './Home';
import Status from './Status';
import Footer from './Footer';
import {pageview} from './gtag'


function App() {
    const [locale, setLocale, localeList, localeMessage] = useLocale();
    return (
        <IntlProvider
            defaultLocale={localeList[0].code}
            key={locale}
            locale={locale}
            messages={localeMessage}
        >
            <React.Fragment>
                <Header locale={locale} setLocale={setLocale} localeList={localeList} />
                <Router>
                    <div>
                        <Routes>
                            <Route path="/" element={<>{pageview('/')}<Home/></>}/>
                            <Route path="/status" element={<>{pageview('/status')}<Status/></>}/>
                        </Routes>
                    </div>
                </Router>
                <Footer/>
            </React.Fragment>
        </IntlProvider>
    );
}

export default App;
