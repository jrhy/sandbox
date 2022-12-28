
To regenerate the Go files from v1/*.proto changes, you need:

```
go install github.com/bufbuild/buf/cmd/buf@v1.9.0
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2
```

