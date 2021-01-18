import React from 'react';
import { useIntl } from 'react-intl';
import { WebsiteUrl } from './Const';

export default function Footer() {
    const intl = useIntl();
    return (
        <footer>
            <div className={"d-flex justify-content-center my-5"}>
            </div>

            <p>{intl.formatMessage({ id: "footer.copyright" })}</p>
        </footer>
    );
}
