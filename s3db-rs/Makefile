all: target/libgob2json.a 

target/libgob2json.a: go_from_rust/libgob2json.go
	cd go_from_rust && ${MAKE}

populate-local-s3db: start-minio-server .setup_s3db
	mc ls myminio/s3db-rs || mb myminio/s3db-rs
	AWS_ACCESS_KEY_ID=minioadmin \
	AWS_SECRET_ACCESS_KEY=minioadmin \
	AWS_REGION=dummy \
	S3_ENDPOINT=http://127.0.0.1:9091 \
	s3db -b s3db-rs set foo=bar

start-minio-server: .setup_minio
	minio server --address 127.0.0.1:9091 --console-address 127.0.0.1:9092 $$(cat .minio_dir) &
	sleep 2
	mc alias set myminio http://127.0.0.1:9091 minioadmin minioadmin

show-env:
	echo export AWS_ACCESS_KEY_ID=minioadmin \
		AWS_SECRET_ACCESS_KEY=minioadmin \
		AWS_REGION=dummy \
		S3_ENDPOINT=http://127.0.0.1:9091

.setup_s3db:
	go install github.com/jrhy/s3db/cmd/s3db
	touch .setup_s3db

.setup_minio:
	go install github.com/minio/minio
	go install github.com/minio/mc
	mktemp -d minio-XXXXXXXXXXX > .minio_dir
	touch .setup_minio

clean:
	-pkill minio
	-rm .setup_minio .setup_s3db .minio_dir

