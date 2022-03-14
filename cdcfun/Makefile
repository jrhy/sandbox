DATA=BCCDC_COVID19_Dashboard_Case_Details.csv
S3DB=s3db --bucket=$$(cat .bucket) --master-key-file=.key -q

all: cdc.version.current cdc.vch.version.current active.txt

fetch::
	#curl -O http://www.bccdc.ca/Health-Info-Site/Documents/${DATA}
	wget -m -np http://www.bccdc.ca/Health-Info-Site/Documents/${DATA}
	cp -rp www.bccdc.ca/Health-Info-Site/Documents/${DATA} ${DATA}

cdc.version.current: /tmp/values
	${S3DB} --prefix=cdc/versionByDate set $$(date +%Y-%m-%d)=$$(cat cdc.version.current)
	${S3DB} --prefix=cdc set $$(cat /tmp/values)
	${S3DB} --prefix=cdc merge | tee cdc.version.current

cdc.vch.version.current: /tmp/values-vch
	${S3DB} --prefix=cdc-vch/versionByDate set $$(date +%Y-%m-%d)=$$(cat cdc.vch.version.current)
	${S3DB} --prefix=cdc-vch set $$(cat /tmp/values-vch)
	${S3DB} --prefix=cdc-vch merge | tee cdc.vch.version.current

/tmp/values: ${DATA}
	cat ${DATA} |tail  +2 | cut -d, -f1 | sort | uniq -c | tr  -d \" \
		| awk '{printf("%s=%s\n", $$2, $$1)}' | sort > /tmp/values

/tmp/values-vch: ${DATA}
	cat ${DATA} | grep 'Vancouver Coastal' | tail  +2 | cut -d, -f1 | sort | uniq -c | tr  -d \" \
		| awk '{printf("%s=%s\n", $$2, $$1)}' | sort > /tmp/values-vch

${DATA}:
	curl -O http://www.bccdc.ca/Health-Info-Site/Documents/${DATA}

diff:
	${S3DB} --prefix=cdc diff $$(cat cdc.version)

clean:
	rm -f ${DATA} /tmp/values cdc.version.current cdc.vch.version.current

active.txt: active.go cdc.vch.version.current
	go run active.go > active.txt
	tail active.txt

