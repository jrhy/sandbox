SHELL=bash

all: target/libgob2json.a 

target/libgob2json.a: go_from_rust/libgob2json.go
	cd go_from_rust && ${MAKE}

S3DB=\
	AWS_ACCESS_KEY_ID=minioadmin \
	AWS_SECRET_ACCESS_KEY=minioadmin \
	AWS_REGION=dummy \
	S3_ENDPOINT=http://127.0.0.1:9091 \
	s3db

populate-local-s3db: start-minio-server .setup_s3db .key
	mc ls myminio/s3db-rs || mc mb myminio/s3db-rs
	${S3DB} -v -b s3db-rs set `let a=0 ; while [ $$a -lt 5000 ] ; do echo foo$$a=bar ; let a=a+1 ; done `
	mc ls myminio/s3db-rs-encrypted || mc mb myminio/s3db-rs-encrypted
	${S3DB} -v -b s3db-rs-encrypted -k .key set foo=bar

start-minio-server: .setup_minio
	MINIO_ROOT_USER=minioadmin MINIO_ROOT_PASSWORD=minioadmin minio server --address 127.0.0.1:9091 --console-address 127.0.0.1:9092 $$(cat .minio_dir) &
	sleep 2
	mc alias set myminio http://127.0.0.1:9091 minioadmin minioadmin

show-env:
	echo export AWS_ACCESS_KEY_ID=minioadmin \
		AWS_SECRET_ACCESS_KEY=minioadmin \
		AWS_REGION=dummy \
		S3_ENDPOINT=http://127.0.0.1:9091

.setup_s3db:
	go install -v github.com/jrhy/s3db/cmd/s3db@latest
	touch .setup_s3db

.key:
	echo keykeykey > .key

.setup_minio: .minio_dir
	go install -v github.com/minio/minio@latest
	go install -v github.com/minio/mc@latest
	touch .setup_minio

.minio_dir:
	mktemp -d /tmp/minio-XXXXXXXXXXX > .minio_dir

clean:
	-pkill minio
	-rm .setup_minio .setup_s3db .minio_dir

