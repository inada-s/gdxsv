import React from 'react';
import { useIntl } from 'react-intl';
import {WebsiteUrl} from './Const';

export default function Footer() {
    const intl = useIntl();
    return (
        <footer>
            <div className={"d-flex justify-content-center my-5"}>
                <a href="https://twitter.com/share?ref_src=twsrc%5Etfw"
                   data-url={WebsiteUrl}
                   className="twitter-share-button mx-2"
                   data-size="large"
                   data-text={intl.formatMessage({ id: "footer.tweet.data-text" })}
                   data-hashtags="gdxsv"
                   data-show-count="false">{intl.formatMessage({ id: "footer.tweet.title" })}</a>
                <div className="fb-share-button mx-2"
                     data-href={WebsiteUrl}
                     data-layout="button"
                     data-size="large">
                    <a target="_blank"
                       rel="noopener noreferrer"
                       href="https://www.facebook.com/sharer/sharer.php?u=https%3A%2F%2Finada-s.github.io%2Fgdxsv%2F&amp;src=sdkpreparse"
                       className="fb-xfbml-parse-ignore">{intl.formatMessage({ id: "footer.facebook.title" })}</a>
                </div>
            </div>

            <p>{intl.formatMessage({ id: "footer.copyright" })}</p>
        </footer>
    );
}
