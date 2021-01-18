import React from 'react';
import { useIntl } from 'react-intl';
import { TwitterButton, FacebookButton, MeweButton, LineButton } from "./ShareButtons";

export default function Footer() {
    const intl = useIntl();
    return (
        <footer>
            <div className={"d-flex justify-content-center my-5"}>
                <TwitterButton />
                <FacebookButton />
                <MeweButton />
                <LineButton />
            </div>
            <p>{intl.formatMessage({ id: "footer.copyright" })}</p>
        </footer>
    );
}
