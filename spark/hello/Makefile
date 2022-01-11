all:
	echo you can 'make run' if ya wanna

build/.setup-spark: archive/spark-3.2.0-bin-hadoop3.2.tgz 
	shasum -a 512 $? | grep ebe51a449ebd070be7d3570931044070e53c23076abad233b3c51d45a7c99326cf55805ee0d573e6eb7d6a67cfef1963cd77d6dc07dd2fd70fd60da9d1f79e5e || echo checksum mismatch
	cd build ; tar xvzf ../$? 
	touch $^

archive/spark-3.2.0-bin-hadoop3.2.tgz: archive
	cd archive ; curl -LO https://dlcdn.apache.org/spark/spark-3.2.0/spark-3.2.0-bin-hadoop3.2.tgz

build: 
	mkdir $^

archive: 
	mkdir $^

clean:
	-rm -fr build archive

run: build build/spark-3.2.0-bin-hadoop3.2
	sbt package
	build/spark-3.2.0-bin-hadoop3.2/bin/spark-submit --class "SimpleApp" \
	  --master 'local[4]' \
	    target/scala-2.12/simple-project_2.12-1.0.jar

tryBloop:
	sbt bloopInstall
	bloop test hello-test -w 


