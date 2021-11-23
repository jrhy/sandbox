all: ../target/libgob2json.a

../target/libgob2json.a: libgob2json.go
	go build -buildmode=c-archive -o ../target/libgob2json.a libgob2json.go

