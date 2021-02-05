mcsfunc
======

Google Cloud Function.

This allow us to allocate mcs (battle server) on demand in any region.

## Local Development and Deploy Memo
1. Create Cloud Function (Web/CUI)
1. Generate service account key (for Development) and Download it as json (Web/CUI)
1. Run `GOOGLE_APPLICATION_CREDENTIALS=/path/to/gcp-key.json npm start` to local development (CUI)
1. Run `gcloud functions deploy mcsfunc --region asia-northeast1 --entry-point cloudFunctionEntryPoint --trigger-http --runtime nodejs12` to deploy (CUI)

## Access Control Memo
1. Generate service account (for function invoker) and its key then download it as json (Web/CUI)
1. Add the account to IAM members with `cloudfunctions.functions.invoke` role. (Web/CUI)
1. Login with the service account and get IDToken to invoke function (App)
    (NOTE: It seems that must specify the base URL of the function as `target_audience`, be careful it is not in the document.)
1. Request function url with `Authorization: Bearer <IDToken>` request header (App).
 