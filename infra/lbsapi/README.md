LbsApi - CloudFunction 
=====

## Develop
```shell
go run cmd/main.go
```

## Deploy
```shell
gcloud functions deploy lbsapi \
  --gen2 \
  --region asia-northeast1 \
  --entry-point FunctionEntryPoint \
  --trigger-http \
  --runtime=go127 \
  --allow-unauthenticated
```

Deployed as a 2nd gen function (1st gen no longer has a supported Go
runtime). Check `gcloud functions runtimes list` before bumping
`--runtime` again; a 1st gen function can't be switched to `--gen2` by
redeploying under the same name — delete it first, then deploy with
`--gen2` to get the same URL back.