import React from 'react';
import {WebsiteUrl} from './Const';

export default function Footer() {
    return (
        <footer>
            <div className={"d-flex justify-content-center my-5"}>
                <a href="https://twitter.com/share?ref_src=twsrc%5Etfw"
                   data-url={WebsiteUrl}
                   className="twitter-share-button mx-2"
                   data-size="large"
                   data-text="連ジDX通信対戦"
                   data-hashtags="gdxsv"
                   data-show-count="false">Tweet</a>
                <div className="fb-share-button mx-2"
                     data-href={WebsiteUrl}
                     data-layout="button"
                     data-size="large">
                    <a target="_blank"
                       rel="noopener noreferrer"
                       href="https://www.facebook.com/sharer/sharer.php?u=https%3A%2F%2Finada-s.github.io%2Fgdxsv%2F&amp;src=sdkpreparse"
                       className="fb-xfbml-parse-ignore">Share</a>
                </div>
            </div>

            <p>© gdxsv project 2020 </p>
        </footer>
    );
}
