Uploader - CloudFunction 
=====

Since gdxsv v1.4.3 this is only a compatibility shim for flycast builds older than
gdxsv-1.9.7: it forwards the posted replay to the lobby's `/lbs/replay_upload`, which
stores it. Newer builds post to the lobby directly.

## Develop
```shell
go run cmd/main.go
```

## Deploy
```shell
gcloud functions deploy uploader \
  --gen2 \
  --region asia-northeast1 \
  --entry-point FunctionEntryPoint \
  --trigger-http \
  --runtime=go127 \
  --allow-unauthenticated
```

Deployed as a 2nd gen function (1st gen has no supported Go runtime). The 1st gen
function of the same name had to be deleted first; the deploy then gets the same
`cloudfunctions.net/uploader` URL back.
