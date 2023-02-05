
.PHONY: clean
clean:
	-rm -f $(LIBS)

start-minio:
	if ! which minio ; then \
		go install -v github.com/minio/minio@latest ; \
		go install -v github.com/minio/mc@latest ; \
	fi
	if ! pgrep minio ; then \
		( MINIO_ROOT_USER=minioadmin \
		MINIO_ROOT_PASSWORD=miniopassword \
		minio server \
			--address 127.0.0.1:9091 \
			--console-address 127.0.0.1:9092 \
			$$(mktemp -d) & ) ; \
		sleep 3 ; \
		mc alias set mysasqlite http://127.0.0.1:9091 minioadmin miniopassword ; \
		mc ls mysasqlite/bucket || mc mb mysasqlite/bucket ; \
		fi

ENV=\
	AWS_ACCESS_KEY_ID=minioadmin \
	AWS_SECRET_ACCESS_KEY=miniopassword \
	AWS_REGION=dummy \
	S3_ENDPOINT='http://127.0.0.1:9091'

run:
	go generate
		echo -cmd "delete from t1;" \
		-cmd "insert into t1 values ('v1','v1b'),('v2','v2b'),('v3','v3b');" \
		-cmd "update t1 set a='v1c' where b='v1b';" \
		-cmd "delete from t1 where b='v2b';" \


	${ENV} sqlite3 \
		-cmd '.load ./sasqlite' \
		-cmd "create virtual table t1 using sasqlite ( \
		    s3_bucket='bucket', \
		    s3_endpoint='http://127.0.0.1:9091', \
		    s3_prefix='t1', \
		    schema='a primary key, b')" \
		-cmd ".mode box" \
		-cmd ".nullvalue NULL" \
		-cmd "select * from t1;" \
		-cmd "create virtual table t2 using sasqlite ( \
		    s3_bucket='bucket', \
		    s3_endpoint='http://127.0.0.1:9091', \
		    s3_prefix='t2', \
		    schema='a primary key, b')"
	false && echo \
		-cmd "create virtual table foo using sasqlite ( \
		    s3_bucket='bucket', \
		    s3_endpoint='http://127.0.0.1:9091', \
		    s3_prefix='foo', \
		    schema='a, b')" \
		-cmd "insert into foo values ('a',current_timestamp);"

