LbsApi - CloudFunction 
=====

## Develop
```shell
go run cmd/main.go
```

## Deploy
```shell
gcloud functions deploy lbsapi \
  --region asia-northeast1 \
  --entry-point FunctionEntryPoint \
  --trigger-http \
  --runtime=go120 \
  --allow-unauthenticated
```